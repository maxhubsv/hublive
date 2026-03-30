#include "config.h"
#include "jwt_token.h"
#include "websocket_transport.h"
#include "signaling_client.h"
#include "peer_connection_agent.h"

#include <winsock2.h>
#include <windows.h>
#include <objbase.h>

#include <cstdio>
#include <cstdlib>
#include <csignal>
#include <ctime>
#include <algorithm>
#include <atomic>
#include <thread>
#include <chrono>

static std::atomic<bool> g_running{true};

void SignalHandler(int sig) {
    printf("\n  Shutting down...\n");
    g_running = false;
}

int main(int argc, char* argv[]) {
    // Disable abort dialog — log and exit cleanly instead of showing popup.
    _set_abort_behavior(0, _WRITE_ABORT_MSG | _CALL_REPORTFAULT);
    SetErrorMode(SEM_FAILCRITICALERRORS | SEM_NOGPFAULTERRORBOX);

    // libwebrtc built with is_debug=false — minimal logging by default.

    // Initialize Winsock (required before any network operations).
    WSADATA wsa_data;
    if (WSAStartup(MAKEWORD(2, 2), &wsa_data) != 0) {
        printf("ERROR: WSAStartup failed\n");
        return 1;
    }

    // Initialize COM for WASAPI audio capture.
    CoInitializeEx(nullptr, COINIT_MULTITHREADED);

    // Resolve config path relative to exe location (for double-click launch).
    std::string config_path = "config.yaml";
    if (argc > 1) {
        config_path = argv[1];
    } else {
        // Get directory of the exe itself.
        char exe_path[MAX_PATH] = {};
        GetModuleFileNameA(nullptr, exe_path, MAX_PATH);
        std::string exe_dir(exe_path);
        auto pos = exe_dir.find_last_of("\\/");
        if (pos != std::string::npos) {
            config_path = exe_dir.substr(0, pos + 1) + "config.yaml";
        }
    }

    AppConfig config = LoadConfig(config_path);

    // Auto-generate unique identity from hostname if not customized.
    if (config.agent.identity == "agent-001" || config.agent.identity == "agent-cpp-001") {
        char hostname[256] = {};
        DWORD size = sizeof(hostname);
        GetComputerNameA(hostname, &size);
        config.agent.identity = std::string("agent-") + hostname;
        config.agent.name = std::string("Screen Agent ") + hostname;
    }

    printf("=== HubLive Screen Agent (C++) ===\n");
    printf("  Server:  %s\n", config.hublive.url.c_str());
    printf("  Room:    %s\n", config.room.name.c_str());
    printf("  Agent:   %s (%s)\n", config.agent.identity.c_str(), config.agent.name.c_str());
    printf("  Capture: monitor=%s fps=%d scale=%.1f\n",
           config.capture.monitor.c_str(), config.capture.fps, config.capture.scale);
    printf("  Audio:   system=%s mic=%s sys_gain=%.1f mic_gain=%.1f\n",
           config.audio.system_enabled ? "on" : "off",
           config.audio.mic_enabled ? "on" : "off",
           config.audio.system_gain, config.audio.mic_gain);
    printf("  Control: enabled=%s mouse=%s keyboard=%s\n",
           config.control.enabled ? "on" : "off",
           config.control.mouse ? "on" : "off",
           config.control.keyboard ? "on" : "off");
    printf("\n");

    // Handle Ctrl+C — registered once, before the retry loop.
    std::signal(SIGINT, SignalHandler);
    std::signal(SIGTERM, SignalHandler);

    // Seed the random number generator for backoff jitter.
    std::srand(static_cast<unsigned>(std::time(nullptr)));

    // Create persistent objects that survive across reconnection attempts.
    // The WebSocket, SignalingClient, and PeerConnectionAgent are created once;
    // on reconnect we Reset/Disconnect them rather than destroying/recreating.
    WebSocketTransport ws;
    SignalingClient signaling(&ws);
    PeerConnectionAgent agent(&signaling, config);

    // One-time WebRTC initialization (factory, threads, screen capture source).
    if (!agent.Initialize()) {
        printf("  ERROR: Failed to initialize WebRTC\n");
        return 1;
    }

    // ----- Retry loop with exponential backoff + jitter -----
    int retry_count = 0;
    const int kMaxBackoffSeconds = 30;

    while (g_running) {
        // --- Backoff delay (skipped on the first attempt) ---
        if (retry_count > 0) {
            int delay = std::min(kMaxBackoffSeconds, (1 << retry_count));
            delay += std::rand() % 3;  // +0..2s jitter
            printf("  Reconnecting in %d seconds (attempt #%d)...\n",
                   delay, retry_count + 1);

            // Sleep in 1-second increments so we can break early on Ctrl+C.
            for (int i = 0; i < delay && g_running; ++i) {
                std::this_thread::sleep_for(std::chrono::seconds(1));
            }
            if (!g_running) break;
        }

        // --- Generate a fresh JWT for each attempt ---
        std::string token = GenerateAccessToken(config);
        printf("  Token generated\n");

        // --- Prepare transport + signaling for a (re-)connection ---
        ws.Reset();
        signaling.Reset();
        agent.Disconnect();  // no-op on the very first iteration (no PC exists yet)

        // --- Register the WebSocket close handler ---
        // We use a local flag so the main-loop below can detect a WS drop
        // independently of PeerConnection state.
        std::atomic<bool> ws_alive{true};

        ws.SetOnClose([&ws_alive](int code, const std::string& reason) {
            printf("  [ws] Closed: %s\n", reason.c_str());
            ws_alive = false;
        });
        ws.SetOnError([](const std::string& error) {
            printf("  [ws] Error: %s\n", error.c_str());
        });

        // --- Attempt WebSocket connection ---
        printf("  Trying HubLive WebSocket to %s...\n", config.hublive.url.c_str());
        bool ws_connected = ws.Connect(config.hublive.url, token);

        bool whip_mode = false;
        bool connection_ok = false;

        if (!ws_connected) {
            printf("  WebSocket failed, falling back to WHIP...\n");

            std::string whip_url = config.hublive.url;
            if (whip_url.substr(0, 5) == "ws://")
                whip_url = "http://" + whip_url.substr(5);
            else if (whip_url.substr(0, 6) == "wss://")
                whip_url = "https://" + whip_url.substr(6);

            agent.FallbackToWhip(whip_url, token);
            whip_mode = true;

            // Check that WHIP actually succeeded (PC was created and is not
            // already in a failed state).
            if (agent.IsConnected() || !agent.IsDisconnected()) {
                printf("  WHIP signaling complete\n\n");
                connection_ok = true;
            } else {
                printf("  WHIP fallback failed\n");
            }
        } else {
            printf("  WebSocket connected\n\n");
            connection_ok = true;
        }

        // --- Reset retry counter only on successful connection ---
        if (connection_ok) {
            retry_count = 0;
        } else {
            // Both WS and WHIP failed — skip the inner loop, go straight
            // to the retry delay.
            if (g_running) {
                printf("\n  Connection failed.  Preparing to retry...\n");
                retry_count++;
            }
            continue;
        }

        // --- Main loop — keepalive ping + disconnect detection ---
        printf("  Streaming... (Ctrl+C to stop)\n\n");
        int ping_counter = 0;
        int pong_miss_counter = 0;
        const int kPingIntervalSec = 10;
        const int kPongTimeoutMisses = 2;  // after 2 missed pongs (20s) treat as dead

        while (g_running) {
            std::this_thread::sleep_for(std::chrono::seconds(1));

            // --- Detect PeerConnection failure ---
            if (agent.IsDisconnected()) {
                printf("  PeerConnection disconnected/failed — will reconnect\n");
                break;
            }

            if (!whip_mode) {
                // --- WebSocket keepalive ping ---
                if (++ping_counter >= kPingIntervalSec) {
                    if (!signaling.SendPing()) {
                        pong_miss_counter++;
                    } else {
                        pong_miss_counter = 0;
                    }
                    ping_counter = 0;
                }

                // --- Detect WebSocket disconnect ---
                if (!ws_alive || !ws.IsConnected()) {
                    printf("  WebSocket disconnected — will reconnect\n");
                    break;
                }

                // --- Detect ping timeout (no successful send for too long) ---
                if (pong_miss_counter >= kPongTimeoutMisses) {
                    printf("  Ping timeout (%d consecutive failures) — will reconnect\n",
                           pong_miss_counter);
                    break;
                }
            }
        }

        // --- If we broke out due to Ctrl+C, skip the retry ---
        if (!g_running) break;

        // --- Prepare for next iteration ---
        printf("\n  Connection lost.  Preparing to retry...\n");
        retry_count++;
    }

    // --- Final cleanup ---
    printf("\n  Stopping...\n");
    agent.Shutdown();
    ws.Close();
    printf("  Agent stopped.\n");

    CoUninitialize();
    WSACleanup();
    return 0;
}
