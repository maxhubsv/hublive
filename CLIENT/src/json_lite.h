/*
 * json_lite.h — Minimal JSON value extractor (header-only).
 *
 * Extracts typed values from flat JSON objects like:
 *   {"t":1,"s":123,"a":1,"x":0.5,"y":0.3,"b":0,"d":0}
 *
 * Only supports flat objects with string/number values (no nesting, no arrays).
 * This is intentionally minimal — just enough for the input control protocol.
 */

#pragma once

#include <string>
#include <cstdlib>
#include <cstdint>
#include <cmath>

namespace json_lite {

// Extract an integer value for the given single-character key.
// Returns default_val if not found.
inline int GetInt(const std::string& json, const char* key, int default_val = 0) {
    // Search for "key":
    std::string search = std::string("\"") + key + "\":";
    size_t pos = json.find(search);
    if (pos == std::string::npos) return default_val;
    pos += search.size();

    // Skip whitespace
    while (pos < json.size() && (json[pos] == ' ' || json[pos] == '\t')) ++pos;
    if (pos >= json.size()) return default_val;

    return std::atoi(json.c_str() + pos);
}

// Extract an unsigned 32-bit integer.
inline uint32_t GetUint32(const std::string& json, const char* key, uint32_t default_val = 0) {
    std::string search = std::string("\"") + key + "\":";
    size_t pos = json.find(search);
    if (pos == std::string::npos) return default_val;
    pos += search.size();

    while (pos < json.size() && (json[pos] == ' ' || json[pos] == '\t')) ++pos;
    if (pos >= json.size()) return default_val;

    return static_cast<uint32_t>(std::strtoul(json.c_str() + pos, nullptr, 10));
}

// Extract a float value.
inline float GetFloat(const std::string& json, const char* key, float default_val = 0.0f) {
    std::string search = std::string("\"") + key + "\":";
    size_t pos = json.find(search);
    if (pos == std::string::npos) return default_val;
    pos += search.size();

    while (pos < json.size() && (json[pos] == ' ' || json[pos] == '\t')) ++pos;
    if (pos >= json.size()) return default_val;

    return std::strtof(json.c_str() + pos, nullptr);
}

// Extract a string value (for debugging — not used in the hot path).
inline std::string GetString(const std::string& json, const char* key,
                             const std::string& default_val = "") {
    std::string search = std::string("\"") + key + "\":\"";
    size_t pos = json.find(search);
    if (pos == std::string::npos) return default_val;
    pos += search.size();

    size_t end = json.find('"', pos);
    if (end == std::string::npos) return default_val;

    return json.substr(pos, end - pos);
}

}  // namespace json_lite
