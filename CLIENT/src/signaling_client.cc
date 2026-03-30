#include "signaling_client.h"
#include <cstdio>

SignalingClient::SignalingClient(WebSocketTransport* transport)
    : transport_(transport) {
    transport_->SetOnMessage([this](const std::vector<uint8_t>& data) {
        OnMessage(data);
    });
}

void SignalingClient::OnMessage(const std::vector<uint8_t>& data) {
    hublive::SignalResponse response;
    if (!response.ParseFromArray(data.data(), static_cast<int>(data.size()))) {
        printf("  [signal] Failed to parse SignalResponse (%zu bytes)\n", data.size());
        return;
    }

    switch (response.message_case()) {
    case hublive::SignalResponse::kJoin:
        printf("  [signal] JoinResponse: room=%s participant=%s\n",
               response.join().room().name().c_str(),
               response.join().participant().identity().c_str());
        if (on_join_) on_join_(response.join());
        break;

    case hublive::SignalResponse::kAnswer:
        printf("  [signal] Answer received\n");
        if (on_answer_) on_answer_(response.answer().sdp());
        break;

    case hublive::SignalResponse::kOffer:
        printf("  [signal] Offer received (subscriber)\n");
        if (on_offer_) on_offer_(response.offer().sdp());
        break;

    case hublive::SignalResponse::kTrickle: {
        auto& trickle = response.trickle();
        printf("  [signal] Trickle ICE (target=%d)\n", trickle.target());
        if (on_trickle_) on_trickle_(trickle.candidateinit(), trickle.target());
        break;
    }

    case hublive::SignalResponse::kTrackPublished:
        printf("  [signal] TrackPublished: cid=%s sid=%s\n",
               response.track_published().cid().c_str(),
               response.track_published().track().sid().c_str());
        if (on_track_published_)
            on_track_published_(response.track_published().cid(),
                               response.track_published().track());
        break;

    case hublive::SignalResponse::kPongResp:
        break;

    case hublive::SignalResponse::kLeave:
        printf("  [signal] Server requested leave\n");
        break;

    default:
        printf("  [signal] Unhandled message type: %d\n", response.message_case());
        break;
    }
}

bool SignalingClient::SendRequest(const hublive::SignalRequest& request) {
    std::string serialized;
    if (!request.SerializeToString(&serialized)) {
        printf("  [signal] Failed to serialize SignalRequest\n");
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
    printf("  [signal] Sending offer (%zu bytes SDP)\n", sdp.size());
    return SendRequest(request);
}

bool SignalingClient::SendAnswer(const std::string& sdp) {
    hublive::SignalRequest request;
    auto* answer = request.mutable_answer();
    answer->set_type("answer");
    answer->set_sdp(sdp);
    printf("  [signal] Sending answer\n");
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
    printf("  [signal] SendAddTrack: cid=%s name=%s %dx%d\n",
           cid.c_str(), name.c_str(), width, height);
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
