#pragma once

#include "signaling_client.h"
#include "screen_capture_source.h"
#include "whip_client.h"
#include "input_injector.h"
#include "audio_capture.h"
#include "audio_resampler.h"
#include "audio_mixer.h"
#include "custom_audio_source.h"
#include "config.h"

#include "api/peer_connection_interface.h"
#include "api/create_peerconnection_factory.h"
#include "api/audio_codecs/builtin_audio_encoder_factory.h"
#include "api/audio_codecs/builtin_audio_decoder_factory.h"
#include "api/video_codecs/builtin_video_encoder_factory.h"
#include "api/video_codecs/builtin_video_decoder_factory.h"
#include "api/data_channel_interface.h"
#include "api/scoped_refptr.h"
#include "api/jsep.h"

#include <string>
#include <memory>
#include <atomic>
#include <thread>
#include <vector>

class PeerConnectionAgent
    : public webrtc::PeerConnectionObserver {
public:
    enum class SignalingMode { HUBLIVE_WS, WHIP };

    PeerConnectionAgent(SignalingClient* signaling, const AppConfig& config);
    ~PeerConnectionAgent() override;

    bool Initialize();
    void Shutdown();
    bool IsConnected() const { return ice_connected_.load(); }

    // Returns true when the PeerConnection has transitioned to a disconnected
    // or failed state after having been connected.  The main retry loop uses
    // this to decide when to trigger a reconnection attempt.
    bool IsDisconnected() const { return disconnected_.load(); }

    // Clean teardown of PeerConnection and tracks, but keeps the factory,
    // threads, and ScreenCaptureSource alive so that a new connection can be
    // established without full re-initialization.
    // After calling Disconnect(), the caller should reset the signaling layer
    // and call Connect() on the WebSocket again — the JoinResponse callback
    // will create a fresh PeerConnection automatically.
    void Disconnect();

    // Fallback: create PeerConnection and publish via WHIP (HTTP POST).
    void FallbackToWhip(const std::string& whip_url, const std::string& token);

private:
    // PeerConnectionObserver overrides
    void OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState new_state) override;
    void OnDataChannel(webrtc::scoped_refptr<webrtc::DataChannelInterface> channel) override;
    void OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState new_state) override;
    void OnIceCandidate(const webrtc::IceCandidate* candidate) override;
    void OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState new_state) override;
    void OnConnectionChange(webrtc::PeerConnectionInterface::PeerConnectionState new_state) override;
    void OnTrack(webrtc::scoped_refptr<webrtc::RtpTransceiverInterface> transceiver) override {}
    void OnRemoveTrack(webrtc::scoped_refptr<webrtc::RtpReceiverInterface> receiver) override {}

    // Signaling callbacks
    void OnJoinResponse(const hublive::JoinResponse& join);
    void OnRemoteAnswer(const std::string& sdp);
    void OnRemoteOffer(const std::string& sdp);
    void OnRemoteTrickle(const std::string& candidate_json, int target);

    // Internal helpers
    bool CreatePeerConnection(const hublive::JoinResponse& join);
    void AddScreenTrack();
    void AddAudioTrack();
    void InitAudioCapture();
    void StartAudioCapture();
    void StopAudioCapture();
    void AudioMixThread();
    void SetupInputDataChannel();
    void OnInputDataChannelMessage(const webrtc::DataBuffer& buffer);
    void CreateOffer();
    void CreateAnswer();

    // Called when CreateOffer/CreateAnswer completes (from inner observer)
    void OnCreateSessionDescSuccess(webrtc::SessionDescriptionInterface* desc);
    void OnCreateSessionDescFailure(webrtc::RTCError error);

    SignalingClient* signaling_;
    AppConfig config_;

    std::unique_ptr<webrtc::Thread> signaling_thread_;
    std::unique_ptr<webrtc::Thread> worker_thread_;
    webrtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> pc_factory_;
    webrtc::scoped_refptr<webrtc::PeerConnectionInterface> peer_connection_;
    webrtc::scoped_refptr<ScreenCaptureSource> screen_source_;

    std::string track_cid_ = "screen_0";
    std::atomic<bool> ice_connected_{false};
    std::atomic<bool> disconnected_{false};  // set when PC goes to failed/disconnected
    bool pending_answer_ = false;  // true when we are creating an answer vs offer

    // WHIP fallback state
    SignalingMode signaling_mode_ = SignalingMode::HUBLIVE_WS;
    std::unique_ptr<hublive::WhipClient> whip_client_;
    hublive::WhipSession whip_session_;

    // Audio capture + mixing
    std::unique_ptr<WasapiCapture> system_capture_;
    std::unique_ptr<WasapiCapture> mic_capture_;
    std::unique_ptr<AudioResampler> system_resampler_;
    std::unique_ptr<AudioResampler> mic_resampler_;
    std::unique_ptr<AudioMixer> audio_mixer_;
    webrtc::scoped_refptr<CustomAudioSource> audio_source_;
    std::string audio_track_cid_ = "audio_0";
    std::atomic<bool> audio_running_{false};
    std::thread audio_mix_thread_;

    // Remote control: DataChannel + input injection
    webrtc::scoped_refptr<webrtc::DataChannelInterface> input_data_channel_;
    std::unique_ptr<webrtc::DataChannelObserver> input_dc_observer_;
    InputInjector input_injector_;

    // LiveKit SFU data channels (for receiving publishData from viewers)
    webrtc::scoped_refptr<webrtc::DataChannelInterface> lossy_dc_;
    webrtc::scoped_refptr<webrtc::DataChannelInterface> reliable_dc_;
    std::unique_ptr<webrtc::DataChannelObserver> lossy_dc_observer_;
    std::unique_ptr<webrtc::DataChannelObserver> reliable_dc_observer_;

    void OnSfuDataChannelMessage(const webrtc::DataBuffer& buffer);
};
