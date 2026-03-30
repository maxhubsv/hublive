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
    std::string monitor = "all";  // "all" or 0-based index ("0", "1", ...)
    float scale = 1.0f;
};

struct AudioConfig {
    bool system_enabled = true;   // Capture system audio (loopback)
    bool mic_enabled = true;      // Capture microphone
    float system_gain = 0.8f;     // System audio volume (0.0-1.0)
    float mic_gain = 1.0f;        // Mic volume (0.0-1.0)
};

struct ControlConfig {
    bool enabled = true;   // Enable remote control
    bool mouse = true;     // Accept mouse input
    bool keyboard = true;  // Accept keyboard input
};

struct AppConfig {
    HubLiveConfig hublive;
    RoomConfig room;
    AgentConfig agent;
    CaptureConfig capture;
    AudioConfig audio;
    ControlConfig control;
};

// Loads config from a YAML-like file. Returns default config on failure.
AppConfig LoadConfig(const std::string& path);
