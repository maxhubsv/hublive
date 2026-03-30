#include "peer_connection_agent.h"

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
#include <functional>
#include <mutex>
#include <condition_variable>

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

    // Create screen capture source (but don't start yet).
    screen_source_ = ScreenCaptureSource::Create(config_.capture.monitor,
                                                  config_.capture.fps);
    if (!screen_source_) {
        printf("[PeerConnectionAgent] Failed to create ScreenCaptureSource\n");
        return false;
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

    printf("[PeerConnectionAgent] Initialized\n");
    return true;
}

void PeerConnectionAgent::Shutdown() {
    if (screen_source_) {
        screen_source_->Stop();
    }
    if (peer_connection_) {
        peer_connection_->Close();
        peer_connection_ = nullptr;
    }
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
    printf("[PeerConnectionAgent] Shut down\n");
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

    // Add screen track (no signaling AddTrack needed for WHIP).
    if (!screen_source_ || !peer_connection_) return;
    screen_source_->Start();

    auto video_track = pc_factory_->CreateVideoTrack(screen_source_, "screen_0");
    if (!video_track) {
        printf("[PeerConnectionAgent] WHIP: Failed to create video track\n");
        return;
    }

    auto add_result = peer_connection_->AddTrack(video_track, {"stream_0"});
    if (!add_result.ok()) {
        printf("[PeerConnectionAgent] WHIP: AddTrack error: %s\n",
               add_result.error().message());
        return;
    }
    printf("[PeerConnectionAgent] WHIP: Screen track added\n");

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

    if (!peer_connection_) {
        printf("[PeerConnectionAgent] No PeerConnection for offer\n");
        return;
    }

    auto desc = webrtc::CreateSessionDescription(webrtc::SdpType::kOffer, sdp);
    if (!desc) {
        printf("[PeerConnectionAgent] Failed to parse offer SDP\n");
        return;
    }

    // Set the remote offer, then create an answer in the completion callback.
    auto observer = webrtc::make_ref_counted<SetRemoteSdpObserver>(
        [this](webrtc::RTCError error) {
            if (error.ok()) {
                CreateAnswer();
            }
        });
    peer_connection_->SetRemoteDescription(std::move(desc), observer);
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

    if (!peer_connection_->AddIceCandidate(candidate.get())) {
        printf("[PeerConnectionAgent] Failed to add ICE candidate\n");
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
    if (!screen_source_ || !peer_connection_) return;

    // Start capturing frames.
    screen_source_->Start();

    // Tell HubLive server about the track before adding it to the PC.
    signaling_->SendAddTrack(
        track_cid_,
        "screen",
        hublive::TrackType::VIDEO,
        hublive::TrackSource::SCREEN_SHARE,
        static_cast<uint32_t>(screen_source_->width()),
        static_cast<uint32_t>(screen_source_->height()));

    // Create a video track from the screen capture source.
    auto video_track = pc_factory_->CreateVideoTrack(screen_source_, "screen_0");
    if (!video_track) {
        printf("[PeerConnectionAgent] Failed to create video track\n");
        return;
    }

    auto add_result = peer_connection_->AddTrack(video_track, {"stream_0"});
    if (!add_result.ok()) {
        printf("[PeerConnectionAgent] AddTrack error: %s\n",
               add_result.error().message());
    } else {
        printf("[PeerConnectionAgent] Screen track added\n");
    }
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
            break;
        case webrtc::PeerConnectionInterface::PeerConnectionState::kDisconnected:
        case webrtc::PeerConnectionInterface::PeerConnectionState::kFailed:
        case webrtc::PeerConnectionInterface::PeerConnectionState::kClosed:
            ice_connected_ = false;
            break;
        default:
            break;
    }
}
