/*
 * HubLive Screen Streaming Agent
 *
 * whip_client.h — WHIP (WebRTC-HTTP Ingestion Protocol) client
 * for publishing a WebRTC stream to HubLive server.
 */

#ifndef AGENT_WHIP_CLIENT_H_
#define AGENT_WHIP_CLIENT_H_

#include <functional>
#include <memory>
#include <string>
#include <vector>

#include "api/peer_connection_interface.h"
#include "api/scoped_refptr.h"

namespace hublive {

struct WhipConfig {
  std::string server_url;   // e.g., "http://localhost:7880"
  std::string bearer_token; // JWT access token
};

struct WhipSession {
  std::string participant_id;  // From Location header
  std::string etag;            // Ice session ID
  std::string answer_sdp;      // Server SDP answer
};

// Performs the WHIP handshake: POST SDP offer, receive SDP answer.
// Uses WinHTTP on Windows for simplicity (no external deps).
class WhipClient {
 public:
  explicit WhipClient(const WhipConfig& config);
  ~WhipClient();

  // Send SDP offer, receive SDP answer. Returns true on success.
  bool Publish(const std::string& offer_sdp, WhipSession* session);

  // Send ICE candidates via PATCH.
  bool TrickleIce(const std::string& participant_id,
                  const std::string& etag,
                  const std::string& ice_fragment);

  // DELETE the session.
  bool Unpublish(const std::string& participant_id);

 private:
  WhipConfig config_;
};

}  // namespace hublive

#endif  // AGENT_WHIP_CLIENT_H_
