#pragma once

#include <string>
#include <cstdint>

struct HubLiveConfig {
    std::string url = "ws://localhost:7880";
    std::string api_key = "key1";
    std::string api_secret = "secret1";
};

struct RoomConfig {
    std::string name = "screen-share";
};

struct AgentConfig {
    std::string identity = "agent-001";
    std::string name = "Screen Agent";
};

struct CaptureConfig {
    int fps = 15;
    int monitor = 0;  // 0-based index (primary monitor)
    float scale = 1.0f;
};

struct AppConfig {
    HubLiveConfig hublive;
    RoomConfig room;
    AgentConfig agent;
    CaptureConfig capture;
};

// Loads config from a YAML-like file. Returns default config on failure.
AppConfig LoadConfig(const std::string& path);
