#include "jwt_token.h"
#include <openssl/hmac.h>
#include <openssl/evp.h>
#include <cstring>
#include <ctime>
#include <string>
#include <vector>

static std::string Base64UrlEncode(const uint8_t* data, size_t len) {
    static const char kTable[] =
        "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/";
    std::string result;
    result.reserve((len + 2) / 3 * 4);

    for (size_t i = 0; i < len; i += 3) {
        uint32_t b = (static_cast<uint32_t>(data[i]) << 16);
        if (i + 1 < len) b |= (static_cast<uint32_t>(data[i + 1]) << 8);
        if (i + 2 < len) b |= static_cast<uint32_t>(data[i + 2]);

        result.push_back(kTable[(b >> 18) & 0x3F]);
        result.push_back(kTable[(b >> 12) & 0x3F]);
        if (i + 1 < len) result.push_back(kTable[(b >> 6) & 0x3F]);
        if (i + 2 < len) result.push_back(kTable[b & 0x3F]);
    }

    // Convert to URL-safe: + -> -, / -> _, remove =
    for (auto& c : result) {
        if (c == '+') c = '-';
        else if (c == '/') c = '_';
    }
    return result;
}

static std::string Base64UrlEncode(const std::string& s) {
    return Base64UrlEncode(reinterpret_cast<const uint8_t*>(s.data()), s.size());
}

static std::string JsonString(const std::string& key, const std::string& value) {
    return "\"" + key + "\":\"" + value + "\"";
}

static std::string JsonInt(const std::string& key, int64_t value) {
    return "\"" + key + "\":" + std::to_string(value);
}

static std::string JsonBool(const std::string& key, bool value) {
    return "\"" + key + "\":" + (value ? "true" : "false");
}

std::string GenerateAccessToken(const AppConfig& config) {
    std::string header = "{\"alg\":\"HS256\",\"typ\":\"JWT\"}";

    int64_t now = static_cast<int64_t>(std::time(nullptr));
    int64_t exp = now + 24 * 3600;

    std::string video_grant = "{" +
        JsonBool("roomJoin", true) + "," +
        JsonString("room", config.room.name) + "," +
        JsonBool("canPublish", true) + "," +
        JsonBool("canSubscribe", false) +
    "}";

    std::string payload = "{" +
        JsonString("iss", config.hublive.api_key) + "," +
        JsonString("sub", config.agent.identity) + "," +
        JsonInt("nbf", now) + "," +
        JsonInt("exp", exp) + "," +
        JsonInt("iat", now) + "," +
        JsonString("identity", config.agent.identity) + "," +
        JsonString("name", config.agent.name) + "," +
        "\"video\":" + video_grant +
    "}";

    std::string header_b64 = Base64UrlEncode(header);
    std::string payload_b64 = Base64UrlEncode(payload);
    std::string signing_input = header_b64 + "." + payload_b64;

    uint8_t signature[32];
    unsigned int sig_len = 0;
    HMAC(EVP_sha256(),
         config.hublive.api_secret.data(),
         static_cast<int>(config.hublive.api_secret.size()),
         reinterpret_cast<const uint8_t*>(signing_input.data()),
         signing_input.size(),
         signature, &sig_len);

    std::string sig_b64 = Base64UrlEncode(signature, sig_len);

    return signing_input + "." + sig_b64;
}
