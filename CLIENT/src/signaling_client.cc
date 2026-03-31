#include "signaling_client.h"
#include "logger.h"

SignalingClient::SignalingClient(WebSocketTransport* transport)
    : transport_(transport) {
    transport_->SetOnMessage([this](const std::vector<uint8_t>& data) {
        OnMessage(data);
    });
}

void SignalingClient::OnMessage(const std::vector<uint8_t>& data) {
    hublive::SignalResponse response;
    if (!response.ParseFromArray(data.data(), static_cast<int>(data.size()))) {
        LogError("signal", "Failed to parse SignalResponse (%zu bytes)", data.size());
        return;
    }

    switch (response.message_case()) {
    case hublive::SignalResponse::kJoin:
        LogInfo("signal", "JoinResponse: room=%s participant=%s",
               response.join().room().name().c_str(),
               response.join().participant().identity().c_str());
        if (on_join_) on_join_(response.join());
        break;

    case hublive::SignalResponse::kAnswer:
        LogInfo("signal", "Answer received");
        if (on_answer_) on_answer_(response.answer().sdp());
        break;

    case hublive::SignalResponse::kOffer:
        LogInfo("signal", "Offer received (subscriber)");
        if (on_offer_) on_offer_(response.offer().sdp());
        break;

    case hublive::SignalResponse::kTrickle: {
        auto& trickle = response.trickle();
        LogDebug("signal", "Trickle ICE (target=%d)", trickle.target());
        if (on_trickle_) on_trickle_(trickle.candidateinit(), trickle.target());
        break;
    }

    case hublive::SignalResponse::kTrackPublished:
        LogInfo("signal", "TrackPublished: cid=%s sid=%s",
               response.track_published().cid().c_str(),
               response.track_published().track().sid().c_str());
        if (on_track_published_)
            on_track_published_(response.track_published().cid(),
                               response.track_published().track());
        break;

    case hublive::SignalResponse::kPongResp:
        break;

    case hublive::SignalResponse::kLeave:
        LogInfo("signal", "Server requested leave");
        break;

    default:
        LogWarn("signal", "Unhandled message type: %d", response.message_case());
        break;
    }
}

bool SignalingClient::SendRequest(const hublive::SignalRequest& request) {
    std::string serialized;
    if (!request.SerializeToString(&serialized)) {
        LogError("signal", "Failed to serialize SignalRequest");
        return false;
    }
    return transport_->Send(reinterpret_cast<const uint8_t*>(serialized.data()),
                           serialized.size());
}

bool SignalingClient::SendOffer(const std::string& sdp) {
    hublive::SignalRequest request;
    auto* offer = request.mutable_offer();
    offer->set_type("offer");
    offer->set_sdp(sdp);
    LogInfo("signal", "Sending offer (%zu bytes SDP)", sdp.size());
    return SendRequest(request);
}

bool SignalingClient::SendAnswer(const std::string& sdp) {
    hublive::SignalRequest request;
    auto* answer = request.mutable_answer();
    answer->set_type("answer");
    answer->set_sdp(sdp);
    LogInfo("signal", "Sending answer");
    return SendRequest(request);
}

bool SignalingClient::SendTrickle(const std::string& candidate_json, int target) {
    hublive::SignalRequest request;
    auto* trickle = request.mutable_trickle();
    trickle->set_candidateinit(candidate_json);
    trickle->set_target(static_cast<hublive::SignalTarget>(target));
    return SendRequest(request);
}

bool SignalingClient::SendAddTrack(const std::string& cid, const std::string& name,
                                    hublive::TrackType type, hublive::TrackSource source,
                                    uint32_t width, uint32_t height) {
    hublive::SignalRequest request;
    auto* add_track = request.mutable_add_track();
    add_track->set_cid(cid);
    add_track->set_name(name);
    add_track->set_type(type);
    add_track->set_source(source);
    add_track->set_width(width);
    add_track->set_height(height);
    LogInfo("signal", "SendAddTrack: cid=%s name=%s %dx%d",
           cid.c_str(), name.c_str(), width, height);
    return SendRequest(request);
}

bool SignalingClient::SendAddAudioTrack(const std::string& cid,
                                         const std::string& name,
                                         hublive::TrackSource source) {
    hublive::SignalRequest request;
    auto* add_track = request.mutable_add_track();
    add_track->set_cid(cid);
    add_track->set_name(name);
    add_track->set_type(hublive::TrackType::AUDIO);
    add_track->set_source(source);
    LogInfo("signal", "SendAddAudioTrack: cid=%s name=%s",
           cid.c_str(), name.c_str());
    return SendRequest(request);
}

bool SignalingClient::SendPing() {
    hublive::SignalRequest request;
    auto* ping = request.mutable_ping_req();
    auto now_ms = std::chrono::duration_cast<std::chrono::milliseconds>(
        std::chrono::system_clock::now().time_since_epoch()).count();
    ping->set_timestamp(now_ms);
    return SendRequest(request);
}

void SignalingClient::Reset() {
    // Re-register the OnMessage handler on the transport. This is necessary
    // because after WebSocketTransport::Reset() + Connect(), the new WebSocket
    // connection needs to route incoming messages back to this SignalingClient.
    transport_->SetOnMessage([this](const std::vector<uint8_t>& data) {
        OnMessage(data);
    });
    // Note: application-level callbacks (on_join_, on_answer_, etc.) are kept
    // intact so the PeerConnectionAgent does not need to re-register them.
}
