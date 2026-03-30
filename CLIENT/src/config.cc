#include "config.h"
#include <fstream>
#include <string>
#include <algorithm>

static std::string Trim(const std::string& s) {
    auto start = s.find_first_not_of(" \t\r\n\"");
    if (start == std::string::npos) return "";
    auto end = s.find_last_not_of(" \t\r\n\"");
    return s.substr(start, end - start + 1);
}

AppConfig LoadConfig(const std::string& path) {
    AppConfig config;
    std::ifstream file(path);
    if (!file.is_open()) {
        printf("Warning: Cannot open %s, using defaults\n", path.c_str());
        return config;
    }

    std::string current_section;
    std::string line;
    while (std::getline(file, line)) {
        auto trimmed = Trim(line);
        if (trimmed.empty() || trimmed[0] == '#') continue;

        if (trimmed.back() == ':' && trimmed.find(' ') == std::string::npos) {
            current_section = trimmed.substr(0, trimmed.size() - 1);
            continue;
        }

        auto colon_pos = trimmed.find(':');
        if (colon_pos == std::string::npos) continue;

        std::string key = Trim(trimmed.substr(0, colon_pos));
        std::string value = Trim(trimmed.substr(colon_pos + 1));

        if (current_section == "hublive") {
            if (key == "url") config.hublive.url = value;
            else if (key == "api_key") config.hublive.api_key = value;
            else if (key == "api_secret") config.hublive.api_secret = value;
        } else if (current_section == "room") {
            if (key == "name") config.room.name = value;
        } else if (current_section == "agent") {
            if (key == "identity") config.agent.identity = value;
            else if (key == "name") config.agent.name = value;
        } else if (current_section == "capture") {
            if (key == "fps") config.capture.fps = std::stoi(value);
            else if (key == "monitor") config.capture.monitor = std::stoi(value) - 1;
            else if (key == "scale") config.capture.scale = std::stof(value);
        } else if (current_section == "audio") {
            if (key == "system_enabled") config.audio.system_enabled = (value == "true" || value == "1");
            else if (key == "mic_enabled") config.audio.mic_enabled = (value == "true" || value == "1");
            else if (key == "system_gain") config.audio.system_gain = std::stof(value);
            else if (key == "mic_gain") config.audio.mic_gain = std::stof(value);
        } else if (current_section == "control") {
            if (key == "enabled") config.control.enabled = (value == "true" || value == "1");
            else if (key == "mouse") config.control.mouse = (value == "true" || value == "1");
            else if (key == "keyboard") config.control.keyboard = (value == "true" || value == "1");
        }
    }

    return config;
}
