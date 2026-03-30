#include "config.h"
#include "jwt_token.h"
#include "websocket_transport.h"
#include "signaling_client.h"
#include "peer_connection_agent.h"

#include <cstdio>
#include <csignal>
#include <atomic>
#include <thread>
#include <chrono>

static std::atomic<bool> g_running{true};

void SignalHandler(int sig) {
    printf("\n  Shutting down...\n");
    g_running = false;
}

int main(int argc, char* argv[]) {
    std::string config_path = "config.yaml";
    if (argc > 1) config_path = argv[1];

    AppConfig config = LoadConfig(config_path);

    printf("=== HubLive Screen Agent (C++) ===\n");
    printf("  Server:  %s\n", config.hublive.url.c_str());
    printf("  Room:    %s\n", config.room.name.c_str());
    printf("  Agent:   %s (%s)\n", config.agent.identity.c_str(), config.agent.name.c_str());
    printf("  Capture: monitor=%d fps=%d scale=%.1f\n",
           config.capture.monitor, config.capture.fps, config.capture.scale);
    printf("\n");

    // 1. Generate JWT token
    std::string token = GenerateAccessToken(config);
    printf("  Token generated\n");

    // 2. Connect WebSocket
    WebSocketTransport ws;
    ws.SetOnClose([](int code, const std::string& reason) {
        printf("  [ws] Closed: %s\n", reason.c_str());
        g_running = false;
    });
    ws.SetOnError([](const std::string& error) {
        printf("  [ws] Error: %s\n", error.c_str());
    });

    SignalingClient signaling(&ws);
    PeerConnectionAgent agent(&signaling, config);

    // 3. Initialize WebRTC
    if (!agent.Initialize()) {
        printf("  ERROR: Failed to initialize WebRTC\n");
        return 1;
    }

    // 4. Connect to server (JoinResponse triggers PeerConnection setup automatically)
    printf("  Trying HubLive WebSocket to %s...\n", config.hublive.url.c_str());
    if (!ws.Connect(config.hublive.url, token)) {
        printf("  WebSocket failed, falling back to WHIP...\n");

        // Convert ws:// to http:// for WHIP endpoint.
        std::string whip_url = config.hublive.url;
        if (whip_url.substr(0, 5) == "ws://")
            whip_url = "http://" + whip_url.substr(5);
        else if (whip_url.substr(0, 6) == "wss://")
            whip_url = "https://" + whip_url.substr(6);

        agent.FallbackToWhip(whip_url, token);
        printf("  WHIP signaling complete\n\n");
    } else {
        printf("  WebSocket connected\n\n");
    }

    // 5. Handle Ctrl+C
    std::signal(SIGINT, SignalHandler);
    std::signal(SIGTERM, SignalHandler);

    // 6. Main loop - keepalive ping + status
    printf("  Streaming... (Ctrl+C to stop)\n\n");
    int ping_counter = 0;
    bool whip_mode = !ws.IsConnected();
    while (g_running) {
        std::this_thread::sleep_for(std::chrono::seconds(1));

        if (!whip_mode) {
            // HubLive WebSocket path: send keepalive pings.
            if (++ping_counter >= 10) {
                signaling.SendPing();
                ping_counter = 0;
            }

            if (!ws.IsConnected()) {
                printf("  WebSocket disconnected, shutting down.\n");
                break;
            }
        }
    }

    // 7. Cleanup
    printf("\n  Stopping...\n");
    agent.Shutdown();
    if (!whip_mode) {
        ws.Close();
    }
    printf("  Agent stopped.\n");

    return 0;
}
