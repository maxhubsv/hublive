#include "peer_connection_agent.h"
#include "json_lite.h"
#include "hublive_models.pb.h"

#include "api/create_peerconnection_factory.h"
#include "api/create_modular_peer_connection_factory.h"
#include "api/enable_media.h"
#include "api/audio_codecs/builtin_audio_encoder_factory.h"
#include "api/audio_codecs/builtin_audio_decoder_factory.h"
#include "api/video_codecs/builtin_video_encoder_factory.h"
#include "api/video_codecs/builtin_video_decoder_factory.h"
#include "api/jsep.h"
#include "api/rtc_error.h"
#include "api/ref_count.h"
#include "api/make_ref_counted.h"
#include "api/set_local_description_observer_interface.h"
#include "api/set_remote_description_observer_interface.h"
#include "rtc_base/thread.h"

#include <cstdio>
#include <cmath>
#include <functional>
#include <mutex>
#include <condition_variable>
#include <vector>

// ---------------------------------------------------------------------------
// Inner ref-counted observer for CreateSessionDescription callbacks.
// PeerConnectionAgent itself is NOT ref-counted (it is PeerConnectionObserver
// which has no ref-counting), so we use a small helper that forwards to it.
// ---------------------------------------------------------------------------
class CreateSdpObserver : public webrtc::CreateSessionDescriptionObserver {
public:
    using SuccessCb = std::function<void(webrtc::SessionDescriptionInterface*)>;
    using FailureCb = std::function<void(webrtc::RTCError)>;

    CreateSdpObserver(SuccessCb on_success, FailureCb on_failure)
        : on_success_(std::move(on_success)),
          on_failure_(std::move(on_failure)) {}

    void OnSuccess(webrtc::SessionDescriptionInterface* desc) override {
        if (on_success_) on_success_(desc);
    }

    void OnFailure(webrtc::RTCError error) override {
        if (on_failure_) on_failure_(std::move(error));
    }

protected:
    ~CreateSdpObserver() override = default;

private:
    SuccessCb on_success_;
    FailureCb on_failure_;
};

// ---------------------------------------------------------------------------
// Inner ref-counted observer for SetLocalDescription
// ---------------------------------------------------------------------------
class SetLocalSdpObserver : public webrtc::SetLocalDescriptionObserverInterface {
public:
    void OnSetLocalDescriptionComplete(webrtc::RTCError error) override {
        if (!error.ok()) {
            printf("[PeerConnectionAgent] SetLocalDescription failed: %s\n",
                   error.message());
        }
    }
protected:
    ~SetLocalSdpObserver() override = default;
};

// ---------------------------------------------------------------------------
// Inner ref-counted observer for SetRemoteDescription
// ---------------------------------------------------------------------------
class SetRemoteSdpObserver : public webrtc::SetRemoteDescriptionObserverInterface {
public:
    using Callback = std::function<void(webrtc::RTCError)>;
    explicit SetRemoteSdpObserver(Callback cb = nullptr) : cb_(std::move(cb)) {}

    void OnSetRemoteDescriptionComplete(webrtc::RTCError error) override {
        if (!error.ok()) {
            printf("[PeerConnectionAgent] SetRemoteDescription failed: %s\n",
                   error.message());
        }
        if (cb_) cb_(std::move(error));
    }
protected:
    ~SetRemoteSdpObserver() override = default;

private:
    Callback cb_;
};

// ===========================================================================
// PeerConnectionAgent
// ===========================================================================

PeerConnectionAgent::PeerConnectionAgent(SignalingClient* signaling,
                                         const AppConfig& config)
    : signaling_(signaling), config_(config) {}

PeerConnectionAgent::~PeerConnectionAgent() {
    Shutdown();
}

bool PeerConnectionAgent::Initialize() {
    // Create dedicated threads for WebRTC internals.
    signaling_thread_ = webrtc::Thread::Create();
    signaling_thread_->SetName("pc_signaling", nullptr);
    if (!signaling_thread_->Start()) {
        printf("[PeerConnectionAgent] Failed to start signaling thread\n");
        return false;
    }

    worker_thread_ = webrtc::Thread::Create();
    worker_thread_->SetName("pc_worker", nullptr);
    if (!worker_thread_->Start()) {
        printf("[PeerConnectionAgent] Failed to start worker thread\n");
        return false;
    }

    // Build factory dependencies with media support.
    webrtc::PeerConnectionFactoryDependencies deps;
    deps.signaling_thread = signaling_thread_.get();
    deps.worker_thread = worker_thread_.get();
    deps.audio_encoder_factory = webrtc::CreateBuiltinAudioEncoderFactory();
    deps.audio_decoder_factory = webrtc::CreateBuiltinAudioDecoderFactory();
    deps.video_encoder_factory = webrtc::CreateBuiltinVideoEncoderFactory();
    deps.video_decoder_factory = webrtc::CreateBuiltinVideoDecoderFactory();
    webrtc::EnableMedia(deps);

    pc_factory_ = webrtc::CreateModularPeerConnectionFactory(std::move(deps));
    if (!pc_factory_) {
        printf("[PeerConnectionAgent] Failed to create PeerConnectionFactory\n");
        return false;
    }

    // Create screen capture source(s) based on monitor config.
    {
        // Enumerate available monitors.
        auto options = webrtc::DesktopCaptureOptions::CreateDefault();
        auto temp_capturer = webrtc::DesktopCapturer::CreateScreenCapturer(options);
        webrtc::DesktopCapturer::SourceList sources;
        if (temp_capturer) temp_capturer->GetSourceList(&sources);

        int monitor_count = static_cast<int>(sources.size());
        printf("[PeerConnectionAgent] Found %d monitor(s)\n", monitor_count);

        std::vector<int> indices;
        if (config_.capture.monitor == "all") {
            for (int i = 0; i < monitor_count; i++) indices.push_back(i);
        } else {
            int idx = std::stoi(config_.capture.monitor);
            // Config uses 1-based, convert to 0-based
            if (idx > 0) idx -= 1;
            if (idx >= 0 && idx < monitor_count) {
                indices.push_back(idx);
            } else if (monitor_count > 0) {
                indices.push_back(0);
            }
        }

        for (int idx : indices) {
            auto src = ScreenCaptureSource::Create(idx, config_.capture.fps);
            if (src) {
                screen_sources_.push_back(src);
                track_cids_.push_back("screen_" + std::to_string(idx));
                printf("[PeerConnectionAgent] Created capture for monitor %d\n", idx);
            }
        }

        if (screen_sources_.empty()) {
            printf("[PeerConnectionAgent] Failed to create any ScreenCaptureSource\n");
            return false;
        }
    }

    // Register signaling callbacks.
    signaling_->SetOnJoin([this](const hublive::JoinResponse& join) {
        OnJoinResponse(join);
    });
    signaling_->SetOnAnswer([this](const std::string& sdp) {
        OnRemoteAnswer(sdp);
    });
    signaling_->SetOnOffer([this](const std::string& sdp) {
        OnRemoteOffer(sdp);
    });
    signaling_->SetOnTrickle([this](const std::string& cand, int target) {
        OnRemoteTrickle(cand, target);
    });

    // Initialize input injector with current screen dimensions.
    if (config_.control.enabled) {
        int sw = GetSystemMetrics(SM_CXSCREEN);
        int sh = GetSystemMetrics(SM_CYSCREEN);
        input_injector_.Init(sw, sh);
        printf("[PeerConnectionAgent] Remote control enabled (screen %dx%d)\n", sw, sh);
    }

    // Initialize audio capture (loopback + mic).
    InitAudioCapture();

    printf("[PeerConnectionAgent] Initialized\n");
    return true;
}

void PeerConnectionAgent::Shutdown() {
    StopAudioCapture();

    // Clean up all DataChannels and their observers.
    if (input_data_channel_) {
        input_data_channel_->UnregisterObserver();
        input_data_channel_->Close();
        input_data_channel_ = nullptr;
    }
    input_dc_observer_.reset();

    if (lossy_dc_) {
        lossy_dc_->UnregisterObserver();
        lossy_dc_->Close();
        lossy_dc_ = nullptr;
    }
    lossy_dc_observer_.reset();

    if (reliable_dc_) {
        reliable_dc_->UnregisterObserver();
        reliable_dc_->Close();
        reliable_dc_ = nullptr;
    }
    reliable_dc_observer_.reset();

    for (auto& src : screen_sources_) {
        if (src) src->Stop();
    }
    if (peer_connection_) {
        peer_connection_->Close();
        peer_connection_ = nullptr;
    }
    audio_source_ = nullptr;
    pc_factory_ = nullptr;

    // Stop threads after releasing all WebRTC objects.
    if (signaling_thread_) {
        signaling_thread_->Stop();
        signaling_thread_.reset();
    }
    if (worker_thread_) {
        worker_thread_->Stop();
        worker_thread_.reset();
    }

    ice_connected_ = false;
    disconnected_ = false;
    printf("[PeerConnectionAgent] Shut down\n");
}

void PeerConnectionAgent::Disconnect() {
    printf("[PeerConnectionAgent] Disconnect — tearing down PeerConnection\n");

    // Stop audio capture threads.
    StopAudioCapture();

    // Close DataChannels before PeerConnection.
    // Unregister observers before closing to avoid dangling callbacks.
    if (input_data_channel_) {
        input_data_channel_->UnregisterObserver();
        input_data_channel_->Close();
        input_data_channel_ = nullptr;
    }
    input_dc_observer_.reset();

    if (lossy_dc_) {
        lossy_dc_->UnregisterObserver();
        lossy_dc_->Close();
        lossy_dc_ = nullptr;
    }
    lossy_dc_observer_.reset();

    if (reliable_dc_) {
        reliable_dc_->UnregisterObserver();
        reliable_dc_->Close();
        reliable_dc_ = nullptr;
    }
    reliable_dc_observer_.reset();

    // Stop screen capture (frames are not useful without a PC).
    // The source objects stay alive — Start() will be called again
    // when AddScreenTrack() runs on the next successful join.
    for (auto& src : screen_sources_) {
        if (src) src->Stop();
    }

    // Close and release the PeerConnection.  This detaches all senders/tracks.
    if (peer_connection_) {
        peer_connection_->Close();
        peer_connection_ = nullptr;
    }

    // Close subscriber PeerConnection.
    if (subscriber_pc_) {
        subscriber_pc_->Close();
        subscriber_pc_ = nullptr;
    }
    subscriber_observer_.reset();

    // Release WHIP state if we were in that mode.
    whip_client_.reset();
    whip_session_ = hublive::WhipSession{};
    signaling_mode_ = SignalingMode::HUBLIVE_WS;

    // Reset flags so the next connection starts cleanly.
    ice_connected_ = false;
    disconnected_ = false;
    pending_answer_ = false;
    pending_subscriber_answer_ = false;

    printf("[PeerConnectionAgent] Disconnect complete — ready for reconnect\n");
}

// ---------------------------------------------------------------------------
// WHIP fallback
// ---------------------------------------------------------------------------

void PeerConnectionAgent::FallbackToWhip(const std::string& whip_url,
                                          const std::string& token) {
    signaling_mode_ = SignalingMode::WHIP;

    printf("[PeerConnectionAgent] WHIP fallback to %s\n", whip_url.c_str());

    // Create WHIP client.
    hublive::WhipConfig whip_config;
    whip_config.server_url = whip_url;
    whip_config.bearer_token = token;
    whip_client_ = std::make_unique<hublive::WhipClient>(whip_config);

    // Create PeerConnection with a default STUN server.
    webrtc::PeerConnectionInterface::RTCConfiguration rtc_config;
    rtc_config.sdp_semantics = webrtc::SdpSemantics::kUnifiedPlan;

    webrtc::PeerConnectionInterface::IceServer stun;
    stun.urls.push_back("stun:stun.l.google.com:19302");
    rtc_config.servers.push_back(stun);

    webrtc::PeerConnectionDependencies pc_deps(this);
    auto result = pc_factory_->CreatePeerConnectionOrError(
        rtc_config, std::move(pc_deps));

    if (!result.ok()) {
        printf("[PeerConnectionAgent] WHIP: CreatePeerConnection error: %s\n",
               result.error().message());
        return;
    }

    peer_connection_ = result.MoveValue();
    printf("[PeerConnectionAgent] WHIP: PeerConnection created\n");

    // Add screen tracks (no signaling AddTrack needed for WHIP).
    if (screen_sources_.empty() || !peer_connection_) return;
    for (size_t i = 0; i < screen_sources_.size(); i++) {
        auto& src = screen_sources_[i];
        src->Start();

        std::string track_id = "screen_" + std::to_string(i);
        auto video_track = pc_factory_->CreateVideoTrack(src, track_id);
        if (!video_track) {
            printf("[PeerConnectionAgent] WHIP: Failed to create video track (monitor %zu)\n", i);
            continue;
        }

        std::string stream_id = "stream_" + std::to_string(i);
        auto add_result = peer_connection_->AddTrack(video_track, {stream_id});
        if (!add_result.ok()) {
            printf("[PeerConnectionAgent] WHIP: AddTrack error (monitor %zu): %s\n",
                   i, add_result.error().message());
        } else {
            printf("[PeerConnectionAgent] WHIP: Screen track added (monitor %zu)\n", i);
        }
    }

    // Add audio track (WHIP mode — no signaling AddTrack needed).
    if (audio_source_) {
        auto audio_track = pc_factory_->CreateAudioTrack("audio_0", audio_source_.get());
        if (audio_track) {
            auto audio_add = peer_connection_->AddTrack(audio_track, {"stream_0"});
            if (audio_add.ok()) {
                printf("[PeerConnectionAgent] WHIP: Audio track added\n");
                StartAudioCapture();
            } else {
                printf("[PeerConnectionAgent] WHIP: Audio AddTrack error: %s\n",
                       audio_add.error().message());
            }
        }
    }

    // Set up input DataChannel for remote control.
    SetupInputDataChannel();

    // Create SDP offer.
    CreateOffer();
}

// ---------------------------------------------------------------------------
// Signaling callbacks
// ---------------------------------------------------------------------------

void PeerConnectionAgent::OnJoinResponse(const hublive::JoinResponse& join) {
    printf("[PeerConnectionAgent] Got JoinResponse (server=%s)\n",
           join.server_version().c_str());

    if (!CreatePeerConnection(join)) {
        printf("[PeerConnectionAgent] Failed to create PeerConnection\n");
        return;
    }

    AddScreenTrack();
    AddAudioTrack();
    SetupInputDataChannel();
    CreateOffer();
}

void PeerConnectionAgent::OnRemoteAnswer(const std::string& sdp) {
    printf("[PeerConnectionAgent] Got remote answer (%zu bytes)\n", sdp.size());

    if (!peer_connection_) {
        printf("[PeerConnectionAgent] No PeerConnection for answer\n");
        return;
    }

    auto desc = webrtc::CreateSessionDescription(webrtc::SdpType::kAnswer, sdp);
    if (!desc) {
        printf("[PeerConnectionAgent] Failed to parse answer SDP\n");
        return;
    }

    auto observer = webrtc::make_ref_counted<SetRemoteSdpObserver>();
    peer_connection_->SetRemoteDescription(std::move(desc), observer);
}

void PeerConnectionAgent::OnRemoteOffer(const std::string& sdp) {
    printf("[PeerConnectionAgent] Got remote offer (subscriber) (%zu bytes)\n",
           sdp.size());

    if (!pc_factory_) {
        printf("[PeerConnectionAgent] No factory for subscriber PC\n");
        return;
    }

    // Create subscriber PeerConnection if not yet created.
    if (!subscriber_pc_) {
        subscriber_observer_ = std::make_unique<SubscriberObserver>(this);

        webrtc::PeerConnectionInterface::RTCConfiguration rtc_config;
        rtc_config.sdp_semantics = webrtc::SdpSemantics::kUnifiedPlan;
        webrtc::PeerConnectionInterface::IceServer stun;
        stun.uri = "stun:stun.l.google.com:19302";
        rtc_config.servers.push_back(stun);

        webrtc::PeerConnectionDependencies deps(subscriber_observer_.get());
        auto result = pc_factory_->CreatePeerConnectionOrError(
            rtc_config, std::move(deps));
        if (!result.ok()) {
            printf("[PeerConnectionAgent] Failed to create subscriber PC\n");
            return;
        }
        subscriber_pc_ = result.MoveValue();
        printf("[PeerConnectionAgent] Subscriber PeerConnection created\n");
    }

    auto desc = webrtc::CreateSessionDescription(webrtc::SdpType::kOffer, sdp);
    if (!desc) {
        printf("[PeerConnectionAgent] Failed to parse subscriber offer SDP\n");
        return;
    }

    // Set the remote offer, then create an answer.
    auto observer = webrtc::make_ref_counted<SetRemoteSdpObserver>(
        [this](webrtc::RTCError error) {
            if (!error.ok()) {
                printf("[PeerConnectionAgent] Subscriber SetRemoteDesc failed: %s\n",
                       error.message());
                return;
            }
            // Create answer on subscriber PC.
            webrtc::PeerConnectionInterface::RTCOfferAnswerOptions opts;
            auto create_obs = webrtc::make_ref_counted<CreateSdpObserver>(
                [this](webrtc::SessionDescriptionInterface* desc) {
                    // Set local description and send answer to server.
                    std::string sdp;
                    desc->ToString(&sdp);
                    printf("[PeerConnectionAgent] Subscriber answer created (%zu bytes)\n", sdp.size());

                    auto set_obs = webrtc::make_ref_counted<SetLocalSdpObserver>();
                    auto answer_desc = webrtc::CreateSessionDescription(webrtc::SdpType::kAnswer, sdp);
                    subscriber_pc_->SetLocalDescription(std::move(answer_desc), set_obs);

                    // Send answer back to server via signaling.
                    signaling_->SendAnswer(sdp);
                },
                [](webrtc::RTCError error) {
                    printf("[PeerConnectionAgent] Subscriber CreateAnswer failed: %s\n",
                           error.message());
                });
            subscriber_pc_->CreateAnswer(create_obs.get(), opts);
        });
    subscriber_pc_->SetRemoteDescription(std::move(desc), observer);
}

void PeerConnectionAgent::OnRemoteTrickle(const std::string& candidate_json,
                                           int target) {
    if (!peer_connection_) return;

    // candidate_json is expected to contain: sdpMid, sdpMLineIndex, candidate
    // We parse it manually since we don't have a JSON library linked.
    // Format from HubLive: {"candidate":"...","sdpMid":"...","sdpMLineIndex":N}
    auto extract_string = [&](const std::string& key) -> std::string {
        std::string search = "\"" + key + "\":\"";
        auto pos = candidate_json.find(search);
        if (pos == std::string::npos) return "";
        pos += search.size();
        auto end = candidate_json.find('"', pos);
        if (end == std::string::npos) return "";
        return candidate_json.substr(pos, end - pos);
    };

    auto extract_int = [&](const std::string& key) -> int {
        std::string search = "\"" + key + "\":";
        auto pos = candidate_json.find(search);
        if (pos == std::string::npos) return 0;
        pos += search.size();
        return std::atoi(candidate_json.c_str() + pos);
    };

    std::string sdp_mid = extract_string("sdpMid");
    int sdp_mline_index = extract_int("sdpMLineIndex");
    std::string candidate_str = extract_string("candidate");

    if (candidate_str.empty()) {
        // End-of-candidates signal
        return;
    }

    webrtc::SdpParseError error;
    auto candidate = webrtc::IceCandidate::Create(
        sdp_mid, sdp_mline_index, candidate_str, &error);
    if (!candidate) {
        printf("[PeerConnectionAgent] Failed to parse ICE candidate: %s\n",
               error.description.c_str());
        return;
    }

    // target=0 is publisher, target=1 is subscriber (LiveKit convention)
    auto* pc = (target == 1 && subscriber_pc_) ? subscriber_pc_.get()
                                                : peer_connection_.get();
    if (pc) {
        if (!pc->AddIceCandidate(candidate.get())) {
            printf("[PeerConnectionAgent] Failed to add ICE candidate (target=%d)\n", target);
        }
    }
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

bool PeerConnectionAgent::CreatePeerConnection(
    const hublive::JoinResponse& join) {

    webrtc::PeerConnectionInterface::RTCConfiguration rtc_config;
    rtc_config.sdp_semantics = webrtc::SdpSemantics::kUnifiedPlan;
    rtc_config.continual_gathering_policy =
        webrtc::PeerConnectionInterface::GATHER_CONTINUALLY;

    // Populate ICE servers from the JoinResponse.
    for (const auto& ice : join.ice_servers()) {
        webrtc::PeerConnectionInterface::IceServer server;
        for (const auto& url : ice.urls()) {
            server.urls.push_back(url);
        }
        server.username = ice.username();
        server.password = ice.credential();
        rtc_config.servers.push_back(server);
    }

    webrtc::PeerConnectionDependencies pc_deps(this);  // 'this' is the observer

    auto result = pc_factory_->CreatePeerConnectionOrError(
        rtc_config, std::move(pc_deps));

    if (!result.ok()) {
        printf("[PeerConnectionAgent] CreatePeerConnection error: %s\n",
               result.error().message());
        return false;
    }

    peer_connection_ = result.MoveValue();
    printf("[PeerConnectionAgent] PeerConnection created\n");
    return true;
}

void PeerConnectionAgent::AddScreenTrack() {
    if (screen_sources_.empty() || !peer_connection_) return;

    for (size_t i = 0; i < screen_sources_.size(); i++) {
        auto& src = screen_sources_[i];
        const auto& cid = track_cids_[i];

        // Start capturing frames.
        src->Start();

        // Tell HubLive server about the track before adding it to the PC.
        signaling_->SendAddTrack(
            cid,
            "screen_" + std::to_string(i),
            hublive::TrackType::VIDEO,
            hublive::TrackSource::SCREEN_SHARE,
            static_cast<uint32_t>(src->width()),
            static_cast<uint32_t>(src->height()));

        // Create a video track from the screen capture source.
        std::string track_id = "screen_" + std::to_string(i);
        auto video_track = pc_factory_->CreateVideoTrack(src, track_id);
        if (!video_track) {
            printf("[PeerConnectionAgent] Failed to create video track for monitor %zu\n", i);
            continue;
        }

        std::string stream_id = "stream_" + std::to_string(i);
        auto add_result = peer_connection_->AddTrack(video_track, {stream_id});
        if (!add_result.ok()) {
            printf("[PeerConnectionAgent] AddTrack error (monitor %zu): %s\n",
                   i, add_result.error().message());
        } else {
            printf("[PeerConnectionAgent] Screen track added (monitor %zu)\n", i);
        }
    }
}

// ---------------------------------------------------------------------------
// Audio capture, mixing, and WebRTC track
// ---------------------------------------------------------------------------

void PeerConnectionAgent::InitAudioCapture() {
    bool any_audio = config_.audio.system_enabled || config_.audio.mic_enabled;
    if (!any_audio) {
        printf("[PeerConnectionAgent] Audio disabled in config\n");
        return;
    }

    // Create the audio source (will be attached to a WebRTC audio track later).
    audio_source_ = webrtc::make_ref_counted<CustomAudioSource>();

    // Create mixer.
    audio_mixer_ = std::make_unique<AudioMixer>();
    audio_mixer_->SetSystemGain(config_.audio.system_gain);
    audio_mixer_->SetMicGain(config_.audio.mic_gain);

    // Initialize system loopback capture.
    if (config_.audio.system_enabled) {
        system_capture_ = std::make_unique<WasapiCapture>();
        if (!system_capture_->Init(true)) {
            printf("[PeerConnectionAgent] System audio init failed — continuing without\n");
            system_capture_.reset();
        } else {
            // Set up resampler: device format -> 48kHz mono.
            system_resampler_ = std::make_unique<AudioResampler>();
            if (!system_resampler_->Init(
                    system_capture_->sample_rate(),
                    system_capture_->channels(),
                    AudioMixer::kSampleRate,
                    AudioMixer::kChannels)) {
                printf("[PeerConnectionAgent] System resampler init failed\n");
                system_capture_.reset();
                system_resampler_.reset();
            } else {
                // Wire capture callback -> resampler -> mixer.
                system_capture_->SetOnData(
                    [this](const float* data, int frames, int channels, int sample_rate) {
                        if (!audio_running_) return;
                        // Resample to 48kHz mono.
                        int max_out = system_resampler_->MaxOutputFrames(frames);
                        std::vector<float> resampled(max_out);
                        int out_frames = 0;
                        system_resampler_->Process(data, frames,
                                                    resampled.data(), &out_frames);
                        if (out_frames > 0) {
                            audio_mixer_->PushSystemData(resampled.data(), out_frames);
                        }
                    });
            }
        }
    }

    // Initialize microphone capture.
    if (config_.audio.mic_enabled) {
        mic_capture_ = std::make_unique<WasapiCapture>();
        if (!mic_capture_->Init(false)) {
            printf("[PeerConnectionAgent] Mic init failed — continuing without\n");
            mic_capture_.reset();
        } else {
            // Set up resampler: device format -> 48kHz mono.
            mic_resampler_ = std::make_unique<AudioResampler>();
            if (!mic_resampler_->Init(
                    mic_capture_->sample_rate(),
                    mic_capture_->channels(),
                    AudioMixer::kSampleRate,
                    AudioMixer::kChannels)) {
                printf("[PeerConnectionAgent] Mic resampler init failed\n");
                mic_capture_.reset();
                mic_resampler_.reset();
            } else {
                // Wire capture callback -> resampler -> mixer.
                mic_capture_->SetOnData(
                    [this](const float* data, int frames, int channels, int sample_rate) {
                        if (!audio_running_) return;
                        int max_out = mic_resampler_->MaxOutputFrames(frames);
                        std::vector<float> resampled(max_out);
                        int out_frames = 0;
                        mic_resampler_->Process(data, frames,
                                                resampled.data(), &out_frames);
                        if (out_frames > 0) {
                            audio_mixer_->PushMicData(resampled.data(), out_frames);
                        }
                    });
            }
        }
    }

    if (!system_capture_ && !mic_capture_) {
        printf("[PeerConnectionAgent] No audio devices available — audio disabled\n");
        audio_source_ = nullptr;
        audio_mixer_.reset();
    } else {
        printf("[PeerConnectionAgent] Audio capture initialized (system=%s mic=%s)\n",
               system_capture_ ? "yes" : "no",
               mic_capture_ ? "yes" : "no");
    }
}

void PeerConnectionAgent::AddAudioTrack() {
    if (!audio_source_ || !peer_connection_) return;

    // Tell HubLive server about the audio track.
    signaling_->SendAddAudioTrack(
        audio_track_cid_,
        "audio-mixed",
        hublive::TrackSource::MICROPHONE);

    // Create a WebRTC audio track from our custom source.
    auto audio_track = pc_factory_->CreateAudioTrack("audio_0", audio_source_.get());
    if (!audio_track) {
        printf("[PeerConnectionAgent] Failed to create audio track\n");
        return;
    }

    auto add_result = peer_connection_->AddTrack(audio_track, {"stream_0"});
    if (!add_result.ok()) {
        printf("[PeerConnectionAgent] Audio AddTrack error: %s\n",
               add_result.error().message());
    } else {
        printf("[PeerConnectionAgent] Audio track added\n");
        // Start capturing and mixing audio.
        StartAudioCapture();
    }
}

void PeerConnectionAgent::StartAudioCapture() {
    if (audio_running_) return;
    if (!audio_mixer_ || !audio_source_) return;

    audio_running_ = true;

    // Start WASAPI capture threads.
    if (system_capture_) system_capture_->Start();
    if (mic_capture_) mic_capture_->Start();

    // Start the mix thread that pulls from the mixer and pushes to WebRTC.
    audio_mix_thread_ = std::thread(&PeerConnectionAgent::AudioMixThread, this);

    printf("[PeerConnectionAgent] Audio capture started\n");
}

void PeerConnectionAgent::StopAudioCapture() {
    if (!audio_running_) return;

    audio_running_ = false;

    // Stop WASAPI capture threads first.
    if (system_capture_) system_capture_->Stop();
    if (mic_capture_) mic_capture_->Stop();

    // Stop the mix thread.
    if (audio_mix_thread_.joinable()) {
        audio_mix_thread_.join();
    }

    printf("[PeerConnectionAgent] Audio capture stopped\n");
}

void PeerConnectionAgent::AudioMixThread() {
    // This thread pulls mixed 10ms frames from the AudioMixer and delivers
    // them to the CustomAudioSource (which forwards to WebRTC sinks).

    constexpr int kFrameSamples = AudioMixer::kFrameSamples;  // 480
    constexpr int kSampleRate = AudioMixer::kSampleRate;      // 48000
    constexpr int kChannels = AudioMixer::kChannels;          // 1

    float mix_buf[kFrameSamples];
    std::vector<int16_t> int16_buf(kFrameSamples);

    while (audio_running_) {
        if (audio_mixer_->GetMixedFrame(mix_buf, kFrameSamples)) {
            // Convert float32 to int16 for WebRTC.
            for (int i = 0; i < kFrameSamples; ++i) {
                float clamped = std::clamp(mix_buf[i], -1.0f, 1.0f);
                int16_buf[i] = static_cast<int16_t>(clamped * 32767.0f);
            }

            // Deliver to WebRTC via CustomAudioSource.
            audio_source_->DeliverData(int16_buf.data(), kFrameSamples,
                                        kSampleRate, kChannels);
        }

        // Sleep for ~10ms (one audio frame duration).
        std::this_thread::sleep_for(std::chrono::milliseconds(10));
    }
}

// ---------------------------------------------------------------------------
// DataChannel for remote input control
// ---------------------------------------------------------------------------

// Inner observer for DataChannel state/message callbacks.
// We use raw pointers back to the PeerConnectionAgent since DataChannelObserver
// is not ref-counted and its lifetime is tied to the DataChannel.
class SimpleDataChannelObserver : public webrtc::DataChannelObserver {
public:
    using MessageCb = std::function<void(const webrtc::DataBuffer&)>;

    explicit SimpleDataChannelObserver(MessageCb on_message)
        : on_message_(std::move(on_message)) {}
    ~SimpleDataChannelObserver() override = default;

    void OnStateChange() override {}
    void OnMessage(const webrtc::DataBuffer& buffer) override {
        if (on_message_) on_message_(buffer);
    }
    void OnBufferedAmountChange(uint64_t sent_data_size) override {}

private:
    MessageCb on_message_;
};

void PeerConnectionAgent::SetupInputDataChannel() {
    if (!config_.control.enabled || !peer_connection_) {
        printf("[PeerConnectionAgent] Remote control disabled or no PC\n");
        return;
    }

    // Create a DataChannel labeled "input". This is a peer-to-peer channel
    // that appears in the SDP offer. In LiveKit/HubLive, however, viewer data
    // typically arrives via SFU-mediated channels ("_lossy"/"_reliable"), so
    // we also handle those in OnDataChannel(). This channel is kept as a
    // fallback for non-SFU (e.g. WHIP) scenarios.
    webrtc::DataChannelInit dc_config;
    dc_config.ordered = true;

    auto result = peer_connection_->CreateDataChannelOrError("input", &dc_config);
    if (!result.ok()) {
        printf("[PeerConnectionAgent] Failed to create input DataChannel: %s\n",
               result.error().message());
        return;
    }

    input_data_channel_ = result.MoveValue();

    // Register observer (member-owned, not a global static).
    input_dc_observer_ = std::make_unique<SimpleDataChannelObserver>(
        [this](const webrtc::DataBuffer& buffer) {
            OnInputDataChannelMessage(buffer);
        });
    input_data_channel_->RegisterObserver(input_dc_observer_.get());

    printf("[PeerConnectionAgent] Input DataChannel created (label=%s)\n",
           input_data_channel_->label().c_str());
}

void PeerConnectionAgent::OnDataChannel(
    webrtc::scoped_refptr<webrtc::DataChannelInterface> channel) {
    printf("[PeerConnectionAgent] Remote DataChannel received: %s\n",
           channel->label().c_str());

    // LiveKit SFU sends data from other participants via channels labeled
    // "_lossy" and "_reliable". These carry DataPacket protobuf messages
    // wrapping UserPacket payloads.
    if (channel->label() == "_lossy") {
        lossy_dc_ = channel;
        lossy_dc_observer_ = std::make_unique<SimpleDataChannelObserver>(
            [this](const webrtc::DataBuffer& buffer) {
                OnSfuDataChannelMessage(buffer);
            });
        lossy_dc_->RegisterObserver(lossy_dc_observer_.get());
        printf("[PeerConnectionAgent] SFU lossy DataChannel attached\n");
        return;
    }

    if (channel->label() == "_reliable") {
        reliable_dc_ = channel;
        reliable_dc_observer_ = std::make_unique<SimpleDataChannelObserver>(
            [this](const webrtc::DataBuffer& buffer) {
                OnSfuDataChannelMessage(buffer);
            });
        reliable_dc_->RegisterObserver(reliable_dc_observer_.get());
        printf("[PeerConnectionAgent] SFU reliable DataChannel attached\n");
        return;
    }

    // Direct peer-to-peer "input" channel (for WHIP fallback or direct DC).
    if (channel->label() == "input" && config_.control.enabled) {
        input_data_channel_ = channel;

        input_dc_observer_ = std::make_unique<SimpleDataChannelObserver>(
            [this](const webrtc::DataBuffer& buffer) {
                OnInputDataChannelMessage(buffer);
            });
        input_data_channel_->RegisterObserver(input_dc_observer_.get());

        printf("[PeerConnectionAgent] Input DataChannel attached\n");
    }
}

void PeerConnectionAgent::OnSfuDataChannelMessage(
    const webrtc::DataBuffer& buffer) {
    // LiveKit SFU data channels carry DataPacket protobuf messages.
    // We extract the UserPacket payload (which contains our JSON input).
    if (!config_.control.enabled) return;

    hublive::DataPacket packet;
    if (!packet.ParseFromArray(buffer.data.data(),
                               static_cast<int>(buffer.data.size()))) {
        // Not a valid DataPacket — ignore silently.
        return;
    }

    if (!packet.has_user()) return;

    const auto& user = packet.user();
    const std::string& payload = user.payload();
    if (payload.empty()) return;

    // Wrap the payload as a DataBuffer and route through the same handler.
    webrtc::DataBuffer json_buf(payload);
    OnInputDataChannelMessage(json_buf);
}

void PeerConnectionAgent::OnInputDataChannelMessage(
    const webrtc::DataBuffer& buffer) {
    if (!config_.control.enabled) return;

    // Parse the JSON message. DataChannel may send text or binary;
    // we handle text (JSON) messages.
    std::string json(reinterpret_cast<const char*>(buffer.data.data()),
                     buffer.data.size());

    int type      = json_lite::GetInt(json, "t", 0);
    uint32_t seq  = json_lite::GetUint32(json, "s", 0);
    int action    = json_lite::GetInt(json, "a", 0);
    float x       = json_lite::GetFloat(json, "x", 0.0f);
    float y       = json_lite::GetFloat(json, "y", 0.0f);
    int button    = json_lite::GetInt(json, "b", 0);
    int delta     = json_lite::GetInt(json, "d", 0);
    int key_code  = json_lite::GetInt(json, "k", 0);
    int modifiers = json_lite::GetInt(json, "m", 0);

    // Check control sub-permissions.
    if (type == 1 && !config_.control.mouse) return;
    if (type == 2 && !config_.control.keyboard) return;

    input_injector_.ProcessMessage(type, seq, action, x, y,
                                   button, delta, key_code, modifiers);
}

void PeerConnectionAgent::CreateOffer() {
    if (!peer_connection_) return;

    pending_answer_ = false;

    webrtc::PeerConnectionInterface::RTCOfferAnswerOptions opts;
    opts.offer_to_receive_audio = 0;
    opts.offer_to_receive_video = 0;

    auto observer = webrtc::make_ref_counted<CreateSdpObserver>(
        [this](webrtc::SessionDescriptionInterface* desc) {
            OnCreateSessionDescSuccess(desc);
        },
        [this](webrtc::RTCError err) {
            OnCreateSessionDescFailure(std::move(err));
        });

    peer_connection_->CreateOffer(observer.get(), opts);
}

void PeerConnectionAgent::CreateAnswer() {
    if (!peer_connection_) return;

    pending_answer_ = true;

    webrtc::PeerConnectionInterface::RTCOfferAnswerOptions opts;
    opts.offer_to_receive_audio = 0;
    opts.offer_to_receive_video = 0;

    auto observer = webrtc::make_ref_counted<CreateSdpObserver>(
        [this](webrtc::SessionDescriptionInterface* desc) {
            OnCreateSessionDescSuccess(desc);
        },
        [this](webrtc::RTCError err) {
            OnCreateSessionDescFailure(std::move(err));
        });

    peer_connection_->CreateAnswer(observer.get(), opts);
}

void PeerConnectionAgent::OnCreateSessionDescSuccess(
    webrtc::SessionDescriptionInterface* desc) {

    // Serialize the SDP before setting it (SetLocalDescription takes ownership).
    std::string sdp_str = desc->ToString();
    std::string type_str = desc->type();

    // Clone and set as local description.
    auto cloned = desc->Clone();
    auto set_observer = webrtc::make_ref_counted<SetLocalSdpObserver>();
    peer_connection_->SetLocalDescription(std::move(cloned), set_observer);

    if (signaling_mode_ == SignalingMode::WHIP) {
        // WHIP path: wait for ICE gathering to complete, then POST full SDP.
        printf("[PeerConnectionAgent] WHIP: Waiting for ICE gathering...\n");

        // Spin-wait up to 5 seconds for ICE gathering to complete.
        for (int i = 0; i < 50; ++i) {
            if (peer_connection_->ice_gathering_state() ==
                webrtc::PeerConnectionInterface::kIceGatheringComplete) {
                break;
            }
            std::this_thread::sleep_for(std::chrono::milliseconds(100));
        }

        // Get the local description with gathered ICE candidates.
        const auto* local_desc = peer_connection_->local_description();
        if (!local_desc) {
            printf("[PeerConnectionAgent] WHIP: No local description available\n");
            return;
        }

        std::string full_sdp = local_desc->ToString();
        printf("[PeerConnectionAgent] WHIP: Posting offer SDP (%zu bytes)\n",
               full_sdp.size());

        // POST offer via WHIP, get answer.
        if (!whip_client_->Publish(full_sdp, &whip_session_)) {
            printf("[PeerConnectionAgent] WHIP: Publish failed\n");
            return;
        }

        printf("[PeerConnectionAgent] WHIP: Got answer SDP (%zu bytes)\n",
               whip_session_.answer_sdp.size());

        // Set remote description with the answer.
        auto answer = webrtc::CreateSessionDescription(
            webrtc::SdpType::kAnswer, whip_session_.answer_sdp);
        if (!answer) {
            printf("[PeerConnectionAgent] WHIP: Failed to parse answer SDP\n");
            return;
        }

        auto remote_observer = webrtc::make_ref_counted<SetRemoteSdpObserver>();
        peer_connection_->SetRemoteDescription(std::move(answer), remote_observer);
        printf("[PeerConnectionAgent] WHIP: Signaling complete\n");
        return;
    }

    // HubLive WebSocket path: send the SDP to the remote peer via signaling.
    if (pending_answer_) {
        printf("[PeerConnectionAgent] Sending answer SDP (%zu bytes)\n",
               sdp_str.size());
        signaling_->SendAnswer(sdp_str);
    } else {
        printf("[PeerConnectionAgent] Sending offer SDP (%zu bytes)\n",
               sdp_str.size());
        signaling_->SendOffer(sdp_str);
    }
}

void PeerConnectionAgent::OnCreateSessionDescFailure(webrtc::RTCError error) {
    printf("[PeerConnectionAgent] CreateSessionDescription failed: %s\n",
           error.message());
}

// ---------------------------------------------------------------------------
// PeerConnectionObserver overrides
// ---------------------------------------------------------------------------

void PeerConnectionAgent::OnSignalingChange(
    webrtc::PeerConnectionInterface::SignalingState new_state) {
    printf("[PeerConnectionAgent] Signaling state: %s\n",
           std::string(webrtc::PeerConnectionInterface::AsString(new_state)).c_str());
}

void PeerConnectionAgent::OnIceGatheringChange(
    webrtc::PeerConnectionInterface::IceGatheringState new_state) {
    printf("[PeerConnectionAgent] ICE gathering state: %s\n",
           std::string(webrtc::PeerConnectionInterface::AsString(new_state)).c_str());
}

void PeerConnectionAgent::OnIceCandidate(const webrtc::IceCandidate* candidate) {
    if (!candidate) return;

    // In WHIP mode, candidates are gathered into the local description.
    // No trickle ICE needed.
    if (signaling_mode_ == SignalingMode::WHIP) return;

    std::string sdp_str = candidate->ToString();
    std::string sdp_mid = candidate->sdp_mid();
    int sdp_mline_index = candidate->sdp_mline_index();

    // Format as JSON for HubLive trickle protocol.
    // Target PUBLISHER = 0 (we are the publisher).
    std::string json = "{\"candidate\":\"" + sdp_str + "\","
                       "\"sdpMid\":\"" + sdp_mid + "\","
                       "\"sdpMLineIndex\":" + std::to_string(sdp_mline_index) + "}";

    signaling_->SendTrickle(json, hublive::SignalTarget::PUBLISHER);
}

void PeerConnectionAgent::OnIceConnectionChange(
    webrtc::PeerConnectionInterface::IceConnectionState new_state) {
    printf("[PeerConnectionAgent] ICE connection state: %s\n",
           std::string(webrtc::PeerConnectionInterface::AsString(new_state)).c_str());
}

void PeerConnectionAgent::OnConnectionChange(
    webrtc::PeerConnectionInterface::PeerConnectionState new_state) {
    printf("[PeerConnectionAgent] Connection state: %s\n",
           std::string(webrtc::PeerConnectionInterface::AsString(new_state)).c_str());

    switch (new_state) {
        case webrtc::PeerConnectionInterface::PeerConnectionState::kConnected:
            ice_connected_ = true;
            disconnected_ = false;
            break;
        case webrtc::PeerConnectionInterface::PeerConnectionState::kDisconnected:
        case webrtc::PeerConnectionInterface::PeerConnectionState::kFailed:
            ice_connected_ = false;
            disconnected_ = true;
            break;
        case webrtc::PeerConnectionInterface::PeerConnectionState::kClosed:
            ice_connected_ = false;
            break;
        default:
            break;
    }
}

// ---------------------------------------------------------------------------
// Subscriber PeerConnection Observer
// ---------------------------------------------------------------------------

void PeerConnectionAgent::SubscriberObserver::OnDataChannel(
    webrtc::scoped_refptr<webrtc::DataChannelInterface> channel) {
    printf("[Subscriber] DataChannel received: %s\n", channel->label().c_str());
    // Route to the main agent's OnDataChannel handler.
    agent_->OnDataChannel(channel);
}

void PeerConnectionAgent::SubscriberObserver::OnIceCandidate(
    const webrtc::IceCandidate* candidate) {
    if (!candidate) return;
    std::string sdp_str = candidate->ToString();
    std::string sdp_mid = candidate->sdp_mid();
    int sdp_mline_index = candidate->sdp_mline_index();

    // Send ICE candidate for subscriber (target=SUBSCRIBER=1).
    std::string json = "{\"candidate\":\"" + sdp_str + "\","
                       "\"sdpMid\":\"" + sdp_mid + "\","
                       "\"sdpMLineIndex\":" + std::to_string(sdp_mline_index) + "}";
    agent_->signaling_->SendTrickle(json, hublive::SignalTarget::SUBSCRIBER);
}
