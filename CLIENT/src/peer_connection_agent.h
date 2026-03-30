#pragma once

#include "signaling_client.h"
#include "screen_capture_source.h"
#include "whip_client.h"
#include "config.h"

#include "api/peer_connection_interface.h"
#include "api/create_peerconnection_factory.h"
#include "api/audio_codecs/builtin_audio_encoder_factory.h"
#include "api/audio_codecs/builtin_audio_decoder_factory.h"
#include "api/video_codecs/builtin_video_encoder_factory.h"
#include "api/video_codecs/builtin_video_decoder_factory.h"
#include "api/scoped_refptr.h"
#include "api/jsep.h"

#include <string>
#include <memory>
#include <atomic>
#include <thread>

class PeerConnectionAgent
    : public webrtc::PeerConnectionObserver {
public:
    enum class SignalingMode { HUBLIVE_WS, WHIP };

    PeerConnectionAgent(SignalingClient* signaling, const AppConfig& config);
    ~PeerConnectionAgent() override;

    bool Initialize();
    void Shutdown();
    bool IsConnected() const { return ice_connected_.load(); }

    // Fallback: create PeerConnection and publish via WHIP (HTTP POST).
    void FallbackToWhip(const std::string& whip_url, const std::string& token);

private:
    // PeerConnectionObserver overrides
    void OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState new_state) override;
    void OnDataChannel(webrtc::scoped_refptr<webrtc::DataChannelInterface> channel) override {}
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
    bool pending_answer_ = false;  // true when we are creating an answer vs offer

    // WHIP fallback state
    SignalingMode signaling_mode_ = SignalingMode::HUBLIVE_WS;
    std::unique_ptr<hublive::WhipClient> whip_client_;
    hublive::WhipSession whip_session_;
};
