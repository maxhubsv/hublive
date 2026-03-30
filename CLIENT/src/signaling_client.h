#pragma once

#include "websocket_transport.h"
#include "config.h"
#include "hublive_rtc.pb.h"
#include "hublive_models.pb.h"

#include <functional>
#include <string>
#include <vector>
#include <mutex>
#include <chrono>

class SignalingClient {
public:
    using OnJoinCallback = std::function<void(const hublive::JoinResponse& join)>;
    using OnAnswerCallback = std::function<void(const std::string& sdp)>;
    using OnOfferCallback = std::function<void(const std::string& sdp)>;
    using OnTrickleCallback = std::function<void(const std::string& candidate_json, int target)>;
    using OnTrackPublishedCallback = std::function<void(const std::string& cid, const hublive::TrackInfo& track)>;

    explicit SignalingClient(WebSocketTransport* transport);

    bool SendOffer(const std::string& sdp);
    bool SendAnswer(const std::string& sdp);
    bool SendTrickle(const std::string& candidate_json, int target);
    bool SendAddTrack(const std::string& cid, const std::string& name,
                      hublive::TrackType type, hublive::TrackSource source,
                      uint32_t width, uint32_t height);
    // Overload for audio tracks (no width/height).
    bool SendAddAudioTrack(const std::string& cid, const std::string& name,
                           hublive::TrackSource source);
    bool SendPing();

    // Reset internal state for reconnection. Re-registers the OnMessage
    // handler on the transport so a new WebSocket connection will be routed
    // to this SignalingClient. Callbacks (OnJoin, OnAnswer, etc.) are preserved.
    void Reset();

    void SetOnJoin(OnJoinCallback cb) { on_join_ = std::move(cb); }
    void SetOnAnswer(OnAnswerCallback cb) { on_answer_ = std::move(cb); }
    void SetOnOffer(OnOfferCallback cb) { on_offer_ = std::move(cb); }
    void SetOnTrickle(OnTrickleCallback cb) { on_trickle_ = std::move(cb); }
    void SetOnTrackPublished(OnTrackPublishedCallback cb) { on_track_published_ = std::move(cb); }

private:
    void OnMessage(const std::vector<uint8_t>& data);
    bool SendRequest(const hublive::SignalRequest& request);

    WebSocketTransport* transport_;

    OnJoinCallback on_join_;
    OnAnswerCallback on_answer_;
    OnOfferCallback on_offer_;
    OnTrickleCallback on_trickle_;
    OnTrackPublishedCallback on_track_published_;
};
