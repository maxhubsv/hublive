#include "config.h"
#include "logger.h"
#include "jwt_token.h"
#include "websocket_transport.h"
#include "signaling_client.h"
#include "peer_connection_agent.h"

#include <winsock2.h>
#include <windows.h>
#include <objbase.h>

#include <cstdlib>
#include <csignal>
#include <ctime>
#include <algorithm>
#include <atomic>
#include <thread>
#include <chrono>

static std::atomic<bool> g_running{true};

void SignalHandler(int sig) {
    LogInfo("main", "Shutdown signal received");
    g_running = false;
}

int main(int argc, char* argv[]) {
    // Disable crash dialogs for production.
    _set_abort_behavior(0, _WRITE_ABORT_MSG | _CALL_REPORTFAULT);
    SetErrorMode(SEM_FAILCRITICALERRORS | SEM_NOGPFAULTERRORBOX);

    // Silence libwebrtc internal logs.
    freopen("NUL", "w", stderr);

    // Initialize Winsock.
    WSADATA wsa_data;
    if (WSAStartup(MAKEWORD(2, 2), &wsa_data) != 0) {
        fprintf(stdout, "FATAL: WSAStartup failed\n");
        return 1;
    }

    // Initialize COM for WASAPI audio.
    CoInitializeEx(nullptr, COINIT_MULTITHREADED);

    // Resolve config path relative to exe location.
    std::string config_path = "config.yaml";
    if (argc > 1) {
        config_path = argv[1];
    } else {
        char exe_path[MAX_PATH] = {};
        GetModuleFileNameA(nullptr, exe_path, MAX_PATH);
        std::string exe_dir(exe_path);
        auto pos = exe_dir.find_last_of("\\/");
        if (pos != std::string::npos) {
            config_path = exe_dir.substr(0, pos + 1) + "config.yaml";
        }
    }

    AppConfig config = LoadConfig(config_path);

    // Setup logger from config.
    Logger::Instance().SetLevel(config.log.level);
    if (!config.log.file.empty()) {
        Logger::Instance().SetFileOutput(config.log.file);
    }

    // Auto-generate unique identity from hostname.
    if (config.agent.identity == "agent-001" || config.agent.identity == "agent-cpp-001") {
        char hostname[256] = {};
        DWORD size = sizeof(hostname);
        GetComputerNameA(hostname, &size);
        config.agent.identity = std::string("agent-") + hostname;
        config.agent.name = std::string("Screen Agent ") + hostname;
    }

    LogInfo("main", "=== HubLive Screen Agent ===");
    LogInfo("main", "Server:  %s", config.hublive.url.c_str());
    LogInfo("main", "Room:    %s", config.room.name.c_str());
    LogInfo("main", "Agent:   %s (%s)", config.agent.identity.c_str(), config.agent.name.c_str());
    LogInfo("main", "Capture: monitor=%s fps=%d", config.capture.monitor.c_str(), config.capture.fps);
    LogInfo("main", "Audio:   system=%s mic=%s", config.audio.system_enabled ? "on" : "off", config.audio.mic_enabled ? "on" : "off");
    LogInfo("main", "Control: %s", config.control.enabled ? "on" : "off");
    LogInfo("main", "Log:     level=%s", config.log.level.c_str());

    std::signal(SIGINT, SignalHandler);
    std::signal(SIGTERM, SignalHandler);
    std::srand(static_cast<unsigned>(std::time(nullptr)));

    WebSocketTransport ws;
    SignalingClient signaling(&ws);
    PeerConnectionAgent agent(&signaling, config);

    if (!agent.Initialize()) {
        LogError("main", "Failed to initialize WebRTC");
        return 1;
    }

    // Retry loop with exponential backoff + jitter.
    int retry_count = 0;
    const int kMaxBackoff = 30;

    while (g_running) {
        if (retry_count > 0) {
            int delay = std::min(kMaxBackoff, (1 << retry_count)) + std::rand() % 3;
            LogInfo("main", "Reconnecting in %ds (attempt #%d)...", delay, retry_count + 1);
            for (int i = 0; i < delay && g_running; ++i)
                std::this_thread::sleep_for(std::chrono::seconds(1));
            if (!g_running) break;
        }

        std::string token = GenerateAccessToken(config);
        LogDebug("main", "JWT token generated");

        ws.Reset();
        signaling.Reset();
        agent.Disconnect();

        std::atomic<bool> ws_alive{true};
        ws.SetOnClose([&ws_alive](int code, const std::string& reason) {
            LogWarn("ws", "Closed: %s (code=%d)", reason.c_str(), code);
            ws_alive = false;
        });
        ws.SetOnError([](const std::string& error) {
            LogError("ws", "%s", error.c_str());
        });

        LogInfo("main", "Connecting to %s...", config.hublive.url.c_str());
        bool ws_connected = ws.Connect(config.hublive.url, token);

        bool whip_mode = false;
        bool connection_ok = false;

        if (!ws_connected) {
            LogWarn("main", "WebSocket failed, trying WHIP fallback...");

            std::string whip_url = config.hublive.url;
            if (whip_url.substr(0, 5) == "ws://")
                whip_url = "http://" + whip_url.substr(5);
            else if (whip_url.substr(0, 6) == "wss://")
                whip_url = "https://" + whip_url.substr(6);

            agent.FallbackToWhip(whip_url, token);
            whip_mode = true;

            if (agent.IsConnected() || !agent.IsDisconnected()) {
                LogInfo("main", "WHIP connected");
                connection_ok = true;
            } else {
                LogError("main", "WHIP fallback failed");
            }
        } else {
            LogInfo("main", "WebSocket connected");
            connection_ok = true;
        }

        if (connection_ok) {
            retry_count = 0;
        } else {
            if (g_running) retry_count++;
            continue;
        }

        LogInfo("main", "Streaming started");
        int ping_counter = 0;
        int pong_miss = 0;

        while (g_running) {
            std::this_thread::sleep_for(std::chrono::seconds(1));

            if (agent.IsDisconnected()) {
                LogWarn("main", "PeerConnection lost");
                break;
            }

            if (!whip_mode) {
                if (++ping_counter >= 10) {
                    if (!signaling.SendPing()) pong_miss++;
                    else pong_miss = 0;
                    ping_counter = 0;
                }
                if (!ws_alive || !ws.IsConnected()) {
                    LogWarn("main", "WebSocket lost");
                    break;
                }
                if (pong_miss >= 2) {
                    LogWarn("main", "Ping timeout (%d misses)", pong_miss);
                    break;
                }
            }
        }

        if (!g_running) break;

        LogInfo("main", "Connection lost, retrying...");
        retry_count++;
    }

    LogInfo("main", "Shutting down...");
    agent.Shutdown();
    ws.Close();
    LogInfo("main", "Agent stopped");

    CoUninitialize();
    WSACleanup();
    return 0;
}
