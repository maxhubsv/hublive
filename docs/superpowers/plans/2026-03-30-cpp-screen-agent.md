# C++ Screen Agent (HubLive) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a standalone C++ Windows executable that captures the screen via libwebrtc's DesktopCapturer, connects to the HubLive server over WebSocket using the HubLive signaling protocol (protobuf), and publishes the screen as a WebRTC video track viewable on the web dashboard.

**Architecture:** The agent is a headless Win32 console application. It generates a JWT token (HS256) for auth, connects via WebSocket to HubLive (port 7880), negotiates WebRTC SDP offer/answer, exchanges ICE candidates, and streams screen frames. The WebRTC layer uses libwebrtc's native C++ API (PeerConnectionFactory, PeerConnection, DesktopCapturer). Signaling uses protobuf over WebSocket. No GUI needed.

**Tech Stack:**
- **libwebrtc** (pre-built at `f:\webrtc-checkout\src\out\Release\`) - WebRTC, BoringSSL, protobuf, libyuv, abseil
- **Win32 API** - WinHTTP for WebSocket, DXGI/GDI for screen capture (via libwebrtc DesktopCapturer)
- **protoc** (from libwebrtc build) - generates C++ from HubLive `.proto` files
- **MSVC 2025** / CMake - build system

**Key Directories:**
- Source: `C:\Users\Admin\Desktop\WCAP\DEVBYHOON\CLIENT\`
- libwebrtc: `f:\webrtc-checkout\src\`
- libwebrtc libs: `f:\webrtc-checkout\src\out\Release\obj\`
- HubLive server: `C:\Users\Admin\Desktop\WCAP\DEVBYHOON\SERVER\`
- Proto files: `C:\Users\Admin\go\pkg\mod\github.com\hublive\protocol@v1.44.1-0.20260211042324-3688e156dc7e\protobufs\`

---

## File Structure

```
CLIENT/
  CMakeLists.txt                    # Build system - links libwebrtc, sets up protobuf generation
  proto/
    hublive_rtc.proto               # Copied from HubLive protocol (signaling messages)
    hublive_models.proto            # Copied from HubLive protocol (data models)
  src/
    main.cc                         # Entry point, config loading, lifecycle management
    config.h                        # Config struct and YAML-like simple parser
    config.cc
    jwt_token.h                     # JWT HS256 token generator (uses BoringSSL)
    jwt_token.cc
    websocket_transport.h           # WinHTTP WebSocket client for signaling
    websocket_transport.cc
    signaling_client.h              # HubLive signaling protocol (protobuf encode/decode)
    signaling_client.cc
    screen_capture_source.h         # DesktopCapturer -> VideoTrackSource adapter
    screen_capture_source.cc
    peer_connection_agent.h         # PeerConnection orchestrator (factory, tracks, SDP, ICE)
    peer_connection_agent.cc
  config.yaml                       # Runtime configuration (same format as Python agent)
```

**Responsibilities:**
- `main.cc` - Loads config, creates components, runs event loop, handles Ctrl+C
- `config.h/cc` - Parses `config.yaml` into a struct (simple key-value parser, no YAML dependency)
- `jwt_token.h/cc` - Generates HubLive-compatible JWT using BoringSSL HMAC-SHA256
- `websocket_transport.h/cc` - WinHTTP-based WebSocket connect/send/recv (binary protobuf frames)
- `signaling_client.h/cc` - Encodes/decodes HubLive SignalRequest/SignalResponse, manages signaling state machine
- `screen_capture_source.h/cc` - Wraps DesktopCapturer, converts BGRA frames to I420, feeds VideoTrackSource
- `peer_connection_agent.h/cc` - Creates PeerConnectionFactory, manages PeerConnection, handles SDP offer/answer and ICE

---

## Task 1: Project Skeleton & CMake Build System

**Files:**
- Create: `CLIENT/CMakeLists.txt`
- Create: `CLIENT/src/main.cc`

This task sets up the CMake project that links against the pre-built libwebrtc static libraries. The build must compile and link a minimal "hello world" executable to prove the toolchain works.

- [ ] **Step 1: Create CMakeLists.txt**

```cmake
cmake_minimum_required(VERSION 3.20)
project(hublive_screen_agent LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# Paths to libwebrtc
set(WEBRTC_SRC "f:/webrtc-checkout/src")
set(WEBRTC_OUT "${WEBRTC_SRC}/out/Release")
set(WEBRTC_OBJ "${WEBRTC_OUT}/obj")

# Protobuf generation from .proto files
set(PROTOC_EXE "${WEBRTC_OUT}/protoc.exe")
set(PROTO_SRC_DIR "${CMAKE_SOURCE_DIR}/proto")
set(PROTO_GEN_DIR "${CMAKE_BINARY_DIR}/proto_gen")
file(MAKE_DIRECTORY ${PROTO_GEN_DIR})

set(PROTO_FILES
    "${PROTO_SRC_DIR}/hublive_models.proto"
    "${PROTO_SRC_DIR}/hublive_rtc.proto"
)

set(PROTO_GEN_SRCS "")
set(PROTO_GEN_HDRS "")
foreach(PROTO_FILE ${PROTO_FILES})
    get_filename_component(PROTO_NAME ${PROTO_FILE} NAME_WE)
    list(APPEND PROTO_GEN_SRCS "${PROTO_GEN_DIR}/${PROTO_NAME}.pb.cc")
    list(APPEND PROTO_GEN_HDRS "${PROTO_GEN_DIR}/${PROTO_NAME}.pb.h")
    add_custom_command(
        OUTPUT "${PROTO_GEN_DIR}/${PROTO_NAME}.pb.cc" "${PROTO_GEN_DIR}/${PROTO_NAME}.pb.h"
        COMMAND ${PROTOC_EXE}
            --proto_path=${PROTO_SRC_DIR}
            --cpp_out=${PROTO_GEN_DIR}
            ${PROTO_FILE}
        DEPENDS ${PROTO_FILE}
        COMMENT "Generating protobuf C++ for ${PROTO_NAME}.proto"
    )
endforeach()

# Source files
set(SOURCES
    src/main.cc
    src/config.cc
    src/jwt_token.cc
    src/websocket_transport.cc
    src/signaling_client.cc
    src/screen_capture_source.cc
    src/peer_connection_agent.cc
    ${PROTO_GEN_SRCS}
)

add_executable(screen_agent ${SOURCES})

target_include_directories(screen_agent PRIVATE
    ${CMAKE_SOURCE_DIR}/src
    ${PROTO_GEN_DIR}
    ${WEBRTC_SRC}
    ${WEBRTC_SRC}/third_party/abseil-cpp
    ${WEBRTC_SRC}/third_party/protobuf/src
    ${WEBRTC_SRC}/third_party/boringssl/src/include
    ${WEBRTC_SRC}/third_party/libyuv/include
    ${WEBRTC_OUT}/gen
)

# Compile definitions matching libwebrtc build
target_compile_definitions(screen_agent PRIVATE
    WEBRTC_WIN
    WIN32_LEAN_AND_MEAN
    NOMINMAX
    UNICODE
    _UNICODE
    _CRT_SECURE_NO_WARNINGS
    WEBRTC_USE_H264
    RTC_ENABLE_VP9
    _SILENCE_ALL_CXX17_DEPRECATION_WARNINGS
    GOOGLE_PROTOBUF_NO_RTTI
)

# Collect all .lib files from the webrtc build
file(GLOB_RECURSE WEBRTC_LIBS "${WEBRTC_OBJ}/*.lib")

target_link_libraries(screen_agent PRIVATE
    ${WEBRTC_LIBS}
    # Windows system libraries
    winhttp.lib
    ws2_32.lib
    secur32.lib
    crypt32.lib
    iphlpapi.lib
    msdmo.lib
    dmoguids.lib
    wmcodecdspuuid.lib
    amstrmid.lib
    strmiids.lib
    d3d11.lib
    dxgi.lib
    dwmapi.lib
    shcore.lib
    winmm.lib
    ole32.lib
    oleaut32.lib
    uuid.lib
    user32.lib
    gdi32.lib
    advapi32.lib
    shell32.lib
    comdlg32.lib
    shlwapi.lib
)

# Disable RTTI and exceptions to match libwebrtc
target_compile_options(screen_agent PRIVATE /GR- /EHsc /W3)
```

- [ ] **Step 2: Create minimal main.cc**

```cpp
// src/main.cc
#include <cstdio>

int main(int argc, char* argv[]) {
    printf("HubLive Screen Agent (C++)\n");
    printf("Build OK\n");
    return 0;
}
```

- [ ] **Step 3: Create stub source files**

Create empty stub files so CMake can configure. Each file just has a comment.

`src/config.h`:
```cpp
#pragma once
// Stub - will be implemented in Task 2
```

`src/config.cc`:
```cpp
#include "config.h"
```

`src/jwt_token.h`:
```cpp
#pragma once
```

`src/jwt_token.cc`:
```cpp
#include "jwt_token.h"
```

`src/websocket_transport.h`:
```cpp
#pragma once
```

`src/websocket_transport.cc`:
```cpp
#include "websocket_transport.h"
```

`src/signaling_client.h`:
```cpp
#pragma once
```

`src/signaling_client.cc`:
```cpp
#include "signaling_client.h"
```

`src/screen_capture_source.h`:
```cpp
#pragma once
```

`src/screen_capture_source.cc`:
```cpp
#include "screen_capture_source.h"
```

`src/peer_connection_agent.h`:
```cpp
#pragma once
```

`src/peer_connection_agent.cc`:
```cpp
#include "peer_connection_agent.h"
```

- [ ] **Step 4: Copy and trim proto files**

Copy the HubLive proto files from:
`C:\Users\Admin\go\pkg\mod\github.com\hublive\protocol@v1.44.1-0.20260211042324-3688e156dc7e\protobufs\`

Only copy `hublive_rtc.proto` and `hublive_models.proto`. Edit them to:
1. Remove `option go_package = ...;` lines
2. Remove any imports other than `hublive_models.proto` and standard google protobuf imports
3. Keep `syntax = "proto3";` and `package hublive;`

For `hublive_rtc.proto`, ensure it imports `hublive_models.proto` (not a full path).
For `hublive_models.proto`, remove imports of `google/protobuf/timestamp.proto` if not needed, or copy the required google proto files.

If google protobuf well-known types are needed, the include path is:
`f:\webrtc-checkout\src\third_party\protobuf\src\`

- [ ] **Step 5: Build and verify**

```bash
cd C:\Users\Admin\Desktop\WCAP\DEVBYHOON\CLIENT
cmake -B build -G "Visual Studio 17 2022" -A x64
cmake --build build --config Release
```

Expected: `build/Release/screen_agent.exe` exists and prints "HubLive Screen Agent (C++)".

Run:
```bash
build\Release\screen_agent.exe
```

Expected output:
```
HubLive Screen Agent (C++)
Build OK
```

- [ ] **Step 6: Commit**

```bash
git add CLIENT/
git commit -m "feat(client): scaffold C++ screen agent with CMake + libwebrtc linkage"
```

---

## Task 2: Config Loader

**Files:**
- Create: `CLIENT/src/config.h`
- Create: `CLIENT/src/config.cc`
- Create: `CLIENT/config.yaml`

Simple config parser that reads the YAML-like config file. Since we want zero external dependencies beyond libwebrtc, we parse a simple key-value format (our config.yaml is simple enough that line-by-line parsing works).

- [ ] **Step 1: Define config structures**

`src/config.h`:
```cpp
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
```

- [ ] **Step 2: Implement config parser**

`src/config.cc`:
```cpp
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
        // Skip comments and empty lines
        auto trimmed = Trim(line);
        if (trimmed.empty() || trimmed[0] == '#') continue;

        // Check for section headers (lines ending with ':' and no value)
        if (trimmed.back() == ':' && trimmed.find(' ') == std::string::npos) {
            current_section = trimmed.substr(0, trimmed.size() - 1);
            continue;
        }

        // Parse key: value
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
            else if (key == "monitor") config.capture.monitor = std::stoi(value) - 1; // Convert 1-based to 0-based
            else if (key == "scale") config.capture.scale = std::stof(value);
        }
    }

    return config;
}
```

- [ ] **Step 3: Create config.yaml**

`CLIENT/config.yaml`:
```yaml
# HubLive Server connection
hublive:
  url: "ws://localhost:7880"
  api_key: "key1"
  api_secret: "secret1"

# Room settings
room:
  name: "screen-share"

# Agent identity
agent:
  identity: "agent-cpp-001"
  name: "Screen Agent C++"

# Screen capture settings
capture:
  fps: 15
  monitor: 1
  scale: 1.0
```

- [ ] **Step 4: Update main.cc to load and display config**

`src/main.cc`:
```cpp
#include "config.h"
#include <cstdio>

int main(int argc, char* argv[]) {
    std::string config_path = "config.yaml";
    if (argc > 1) config_path = argv[1];

    AppConfig config = LoadConfig(config_path);

    printf("HubLive Screen Agent (C++)\n");
    printf("  Server:  %s\n", config.hublive.url.c_str());
    printf("  Room:    %s\n", config.room.name.c_str());
    printf("  Agent:   %s (%s)\n", config.agent.identity.c_str(), config.agent.name.c_str());
    printf("  Capture: monitor=%d fps=%d scale=%.1f\n",
           config.capture.monitor, config.capture.fps, config.capture.scale);

    return 0;
}
```

- [ ] **Step 5: Build and test**

```bash
cd C:\Users\Admin\Desktop\WCAP\DEVBYHOON\CLIENT
cmake --build build --config Release
cd build\Release
copy ..\..\config.yaml .
screen_agent.exe
```

Expected:
```
HubLive Screen Agent (C++)
  Server:  ws://localhost:7880
  Room:    screen-share
  Agent:   agent-cpp-001 (Screen Agent C++)
  Capture: monitor=0 fps=15 scale=1.0
```

- [ ] **Step 6: Commit**

```bash
git add CLIENT/src/config.h CLIENT/src/config.cc CLIENT/config.yaml CLIENT/src/main.cc
git commit -m "feat(client): add config loader with YAML-like parsing"
```

---

## Task 3: JWT Token Generator

**Files:**
- Create: `CLIENT/src/jwt_token.h`
- Create: `CLIENT/src/jwt_token.cc`

Generates HubLive-compatible JWT access tokens using BoringSSL (already linked from libwebrtc) for HMAC-SHA256.

**JWT structure:**
- Header: `{"alg":"HS256","typ":"JWT"}`
- Payload: standard claims (iss, sub, exp, nbf, iat) + HubLive grants (video.roomJoin, video.room, video.canPublish, etc.)
- Signature: HMAC-SHA256 of `base64url(header).base64url(payload)` keyed with `api_secret`

- [ ] **Step 1: Define JWT interface**

`src/jwt_token.h`:
```cpp
#pragma once

#include "config.h"
#include <string>

// Generates a HubLive-compatible JWT access token.
// The token allows joining the specified room with publish permissions.
std::string GenerateAccessToken(const AppConfig& config);
```

- [ ] **Step 2: Implement JWT generation**

`src/jwt_token.cc`:
```cpp
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

// Minimal JSON builder - no external dependency needed for this simple structure
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
    // Header
    std::string header = "{\"alg\":\"HS256\",\"typ\":\"JWT\"}";

    // Payload
    int64_t now = static_cast<int64_t>(std::time(nullptr));
    int64_t exp = now + 24 * 3600;  // 24 hours validity

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

    // Encode header and payload
    std::string header_b64 = Base64UrlEncode(header);
    std::string payload_b64 = Base64UrlEncode(payload);
    std::string signing_input = header_b64 + "." + payload_b64;

    // HMAC-SHA256 signature using BoringSSL
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
```

- [ ] **Step 3: Test token generation in main.cc**

Update `src/main.cc`:
```cpp
#include "config.h"
#include "jwt_token.h"
#include <cstdio>

int main(int argc, char* argv[]) {
    std::string config_path = "config.yaml";
    if (argc > 1) config_path = argv[1];

    AppConfig config = LoadConfig(config_path);

    printf("HubLive Screen Agent (C++)\n");
    printf("  Server:  %s\n", config.hublive.url.c_str());
    printf("  Room:    %s\n", config.room.name.c_str());
    printf("  Agent:   %s (%s)\n", config.agent.identity.c_str(), config.agent.name.c_str());
    printf("  Capture: monitor=%d fps=%d scale=%.1f\n",
           config.capture.monitor, config.capture.fps, config.capture.scale);

    std::string token = GenerateAccessToken(config);
    printf("  Token:   %s...%s\n",
           token.substr(0, 20).c_str(),
           token.substr(token.size() - 10).c_str());

    return 0;
}
```

- [ ] **Step 4: Build and test**

```bash
cmake --build build --config Release
build\Release\screen_agent.exe
```

Expected: Token is printed as a JWT string (three base64url segments separated by dots). You can verify it at jwt.io by pasting it and entering `secret1` as the verification key.

- [ ] **Step 5: Commit**

```bash
git add CLIENT/src/jwt_token.h CLIENT/src/jwt_token.cc CLIENT/src/main.cc
git commit -m "feat(client): JWT token generator using BoringSSL HMAC-SHA256"
```

---

## Task 4: WebSocket Transport

**Files:**
- Create: `CLIENT/src/websocket_transport.h`
- Create: `CLIENT/src/websocket_transport.cc`

Uses WinHTTP WebSocket API (built into Windows, no external dependency) for connecting to the HubLive server's WebSocket endpoint.

- [ ] **Step 1: Define WebSocket interface**

`src/websocket_transport.h`:
```cpp
#pragma once

#include <windows.h>
#include <winhttp.h>
#include <string>
#include <vector>
#include <functional>
#include <thread>
#include <atomic>
#include <mutex>

class WebSocketTransport {
public:
    using OnMessageCallback = std::function<void(const std::vector<uint8_t>& data)>;
    using OnCloseCallback = std::function<void(int code, const std::string& reason)>;
    using OnErrorCallback = std::function<void(const std::string& error)>;

    WebSocketTransport();
    ~WebSocketTransport();

    // Connect to ws://host:port/path with optional Authorization header.
    // Returns true on success.
    bool Connect(const std::string& url, const std::string& auth_token);

    // Send binary message (protobuf frame).
    bool Send(const std::vector<uint8_t>& data);
    bool Send(const uint8_t* data, size_t len);

    // Close the connection.
    void Close();

    bool IsConnected() const { return connected_.load(); }

    void SetOnMessage(OnMessageCallback cb) { on_message_ = std::move(cb); }
    void SetOnClose(OnCloseCallback cb) { on_close_ = std::move(cb); }
    void SetOnError(OnErrorCallback cb) { on_error_ = std::move(cb); }

private:
    void RecvLoop();
    void Cleanup();

    HINTERNET session_ = nullptr;
    HINTERNET connection_ = nullptr;
    HINTERNET request_ = nullptr;
    HINTERNET websocket_ = nullptr;

    std::thread recv_thread_;
    std::atomic<bool> connected_{false};
    std::mutex send_mutex_;

    OnMessageCallback on_message_;
    OnCloseCallback on_close_;
    OnErrorCallback on_error_;
};
```

- [ ] **Step 2: Implement WebSocket transport**

`src/websocket_transport.cc`:
```cpp
#include "websocket_transport.h"
#include <cstdio>
#include <sstream>

#pragma comment(lib, "winhttp.lib")

// Parse ws://host:port/path
static bool ParseWsUrl(const std::string& url,
                       std::wstring& host, INTERNET_PORT& port, std::wstring& path) {
    std::string work = url;
    bool is_secure = false;

    if (work.find("wss://") == 0) {
        work = work.substr(6);
        is_secure = true;
        port = 443;
    } else if (work.find("ws://") == 0) {
        work = work.substr(5);
        port = 80;
    } else {
        return false;
    }

    // Split host:port and path
    auto slash_pos = work.find('/');
    std::string host_port = (slash_pos != std::string::npos) ? work.substr(0, slash_pos) : work;
    std::string path_str = (slash_pos != std::string::npos) ? work.substr(slash_pos) : "/rtc";

    auto colon_pos = host_port.find(':');
    std::string host_str;
    if (colon_pos != std::string::npos) {
        host_str = host_port.substr(0, colon_pos);
        port = static_cast<INTERNET_PORT>(std::stoi(host_port.substr(colon_pos + 1)));
    } else {
        host_str = host_port;
    }

    host.assign(host_str.begin(), host_str.end());
    path.assign(path_str.begin(), path_str.end());
    return true;
}

static std::wstring Utf8ToWide(const std::string& s) {
    if (s.empty()) return L"";
    int len = MultiByteToWideChar(CP_UTF8, 0, s.c_str(), -1, nullptr, 0);
    std::wstring ws(len - 1, 0);
    MultiByteToWideChar(CP_UTF8, 0, s.c_str(), -1, &ws[0], len);
    return ws;
}

WebSocketTransport::WebSocketTransport() {}

WebSocketTransport::~WebSocketTransport() {
    Close();
}

bool WebSocketTransport::Connect(const std::string& url, const std::string& auth_token) {
    std::wstring host;
    INTERNET_PORT port;
    std::wstring path;

    if (!ParseWsUrl(url, host, port, path)) {
        if (on_error_) on_error_("Invalid WebSocket URL: " + url);
        return false;
    }

    // Append /rtc if path is just "/"
    if (path == L"/") path = L"/rtc";

    // Add query parameter for protocol version
    path += L"?protocol=16&auto_subscribe=false&sdk=cpp";

    session_ = WinHttpOpen(L"HubLiveAgent/1.0",
                           WINHTTP_ACCESS_TYPE_NO_PROXY,
                           WINHTTP_NO_PROXY_NAME,
                           WINHTTP_NO_PROXY_BYPASS, 0);
    if (!session_) {
        if (on_error_) on_error_("WinHttpOpen failed: " + std::to_string(GetLastError()));
        return false;
    }

    connection_ = WinHttpConnect(session_, host.c_str(), port, 0);
    if (!connection_) {
        if (on_error_) on_error_("WinHttpConnect failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    request_ = WinHttpOpenRequest(connection_, L"GET", path.c_str(),
                                   nullptr, WINHTTP_NO_REFERER,
                                   WINHTTP_DEFAULT_ACCEPT_TYPES, 0);
    if (!request_) {
        if (on_error_) on_error_("WinHttpOpenRequest failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    // Set WebSocket upgrade option
    DWORD option = WINHTTP_OPTION_UPGRADE_TO_WEB_SOCKET;
    if (!WinHttpSetOption(request_, WINHTTP_OPTION_UPGRADE_TO_WEB_SOCKET, nullptr, 0)) {
        if (on_error_) on_error_("Failed to set WebSocket upgrade option");
        Cleanup();
        return false;
    }

    // Add Authorization header
    std::wstring auth_header = L"Authorization: Bearer " + Utf8ToWide(auth_token);
    WinHttpAddRequestHeaders(request_, auth_header.c_str(), -1L, WINHTTP_ADDREQ_FLAG_ADD);

    // Send HTTP request
    if (!WinHttpSendRequest(request_, WINHTTP_NO_ADDITIONAL_HEADERS, 0,
                            WINHTTP_NO_REQUEST_DATA, 0, 0, 0)) {
        if (on_error_) on_error_("WinHttpSendRequest failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    if (!WinHttpReceiveResponse(request_, nullptr)) {
        if (on_error_) on_error_("WinHttpReceiveResponse failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    // Complete WebSocket upgrade
    websocket_ = WinHttpWebSocketCompleteUpgrade(request_, 0);
    if (!websocket_) {
        if (on_error_) on_error_("WebSocket upgrade failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    // Close the request handle (no longer needed after upgrade)
    WinHttpCloseHandle(request_);
    request_ = nullptr;

    connected_ = true;

    // Start receive thread
    recv_thread_ = std::thread(&WebSocketTransport::RecvLoop, this);

    return true;
}

bool WebSocketTransport::Send(const std::vector<uint8_t>& data) {
    return Send(data.data(), data.size());
}

bool WebSocketTransport::Send(const uint8_t* data, size_t len) {
    if (!connected_ || !websocket_) return false;

    std::lock_guard<std::mutex> lock(send_mutex_);
    DWORD err = WinHttpWebSocketSend(websocket_,
                                      WINHTTP_WEB_SOCKET_BINARY_MESSAGE_BUFFER_TYPE,
                                      (PVOID)data, static_cast<DWORD>(len));
    if (err != ERROR_SUCCESS) {
        printf("WebSocket send error: %lu\n", err);
        return false;
    }
    return true;
}

void WebSocketTransport::RecvLoop() {
    std::vector<uint8_t> buffer(64 * 1024);  // 64KB receive buffer
    std::vector<uint8_t> message;

    while (connected_ && websocket_) {
        DWORD bytes_read = 0;
        WINHTTP_WEB_SOCKET_BUFFER_TYPE buffer_type;

        DWORD err = WinHttpWebSocketReceive(websocket_,
                                             buffer.data(),
                                             static_cast<DWORD>(buffer.size()),
                                             &bytes_read, &buffer_type);
        if (err != ERROR_SUCCESS) {
            if (connected_) {
                connected_ = false;
                if (on_error_) on_error_("WebSocket recv error: " + std::to_string(err));
            }
            break;
        }

        if (buffer_type == WINHTTP_WEB_SOCKET_CLOSE_BUFFER_TYPE) {
            connected_ = false;
            if (on_close_) on_close_(0, "Server closed connection");
            break;
        }

        if (buffer_type == WINHTTP_WEB_SOCKET_BINARY_MESSAGE_BUFFER_TYPE ||
            buffer_type == WINHTTP_WEB_SOCKET_BINARY_FRAGMENT_BUFFER_TYPE) {
            message.insert(message.end(), buffer.begin(), buffer.begin() + bytes_read);

            if (buffer_type == WINHTTP_WEB_SOCKET_BINARY_MESSAGE_BUFFER_TYPE) {
                if (on_message_) on_message_(message);
                message.clear();
            }
        }
    }
}

void WebSocketTransport::Close() {
    connected_ = false;

    if (websocket_) {
        WinHttpWebSocketClose(websocket_, WINHTTP_WEB_SOCKET_SUCCESS_CLOSE_STATUS, nullptr, 0);
    }

    if (recv_thread_.joinable()) {
        recv_thread_.join();
    }

    Cleanup();
}

void WebSocketTransport::Cleanup() {
    if (websocket_) { WinHttpCloseHandle(websocket_); websocket_ = nullptr; }
    if (request_) { WinHttpCloseHandle(request_); request_ = nullptr; }
    if (connection_) { WinHttpCloseHandle(connection_); connection_ = nullptr; }
    if (session_) { WinHttpCloseHandle(session_); session_ = nullptr; }
}
```

- [ ] **Step 3: Test WebSocket connection to HubLive**

Update `src/main.cc`:
```cpp
#include "config.h"
#include "jwt_token.h"
#include "websocket_transport.h"
#include <cstdio>
#include <thread>
#include <chrono>

int main(int argc, char* argv[]) {
    std::string config_path = "config.yaml";
    if (argc > 1) config_path = argv[1];

    AppConfig config = LoadConfig(config_path);

    printf("HubLive Screen Agent (C++)\n");
    printf("  Server:  %s\n", config.hublive.url.c_str());
    printf("  Room:    %s\n", config.room.name.c_str());
    printf("  Agent:   %s\n", config.agent.identity.c_str());

    std::string token = GenerateAccessToken(config);
    printf("  Token:   %s...\n", token.substr(0, 30).c_str());

    WebSocketTransport ws;
    ws.SetOnMessage([](const std::vector<uint8_t>& data) {
        printf("  WS recv: %zu bytes\n", data.size());
    });
    ws.SetOnClose([](int code, const std::string& reason) {
        printf("  WS closed: %d %s\n", code, reason.c_str());
    });
    ws.SetOnError([](const std::string& error) {
        printf("  WS error: %s\n", error.c_str());
    });

    printf("  Connecting...\n");
    if (ws.Connect(config.hublive.url, token)) {
        printf("  Connected! Waiting for messages...\n");
        std::this_thread::sleep_for(std::chrono::seconds(3));
        ws.Close();
        printf("  Disconnected.\n");
    } else {
        printf("  Failed to connect.\n");
    }

    return 0;
}
```

- [ ] **Step 4: Build and test with HubLive server running**

Start the HubLive server (`hublive-server --dev`), then:

```bash
cmake --build build --config Release
build\Release\screen_agent.exe
```

Expected: "Connected!" and at least one "WS recv" message (the JoinResponse).

- [ ] **Step 5: Commit**

```bash
git add CLIENT/src/websocket_transport.h CLIENT/src/websocket_transport.cc CLIENT/src/main.cc
git commit -m "feat(client): WinHTTP WebSocket transport for HubLive signaling"
```

---

## Task 5: HubLive Signaling Client

**Files:**
- Create: `CLIENT/src/signaling_client.h`
- Create: `CLIENT/src/signaling_client.cc`

Encodes/decodes HubLive protobuf SignalRequest/SignalResponse messages. Manages the signaling state machine: receive JoinResponse, send AddTrack, send Offer, receive Answer, exchange ICE candidates.

- [ ] **Step 1: Define signaling client interface**

`src/signaling_client.h`:
```cpp
#pragma once

#include "websocket_transport.h"
#include "config.h"
#include "hublive_rtc.pb.h"
#include "hublive_models.pb.h"

#include <functional>
#include <string>
#include <vector>
#include <mutex>

class SignalingClient {
public:
    using OnJoinCallback = std::function<void(const hublive::JoinResponse& join)>;
    using OnAnswerCallback = std::function<void(const std::string& sdp)>;
    using OnOfferCallback = std::function<void(const std::string& sdp)>;
    using OnTrickleCallback = std::function<void(const std::string& candidate_json, int target)>;
    using OnTrackPublishedCallback = std::function<void(const std::string& cid, const hublive::TrackInfo& track)>;

    explicit SignalingClient(WebSocketTransport* transport);

    // Send methods
    bool SendOffer(const std::string& sdp);
    bool SendAnswer(const std::string& sdp);
    bool SendTrickle(const std::string& candidate_json, int target);
    bool SendAddTrack(const std::string& cid, const std::string& name,
                      hublive::TrackType type, hublive::TrackSource source,
                      uint32_t width, uint32_t height);
    bool SendPing();

    // Register callbacks
    void SetOnJoin(OnJoinCallback cb) { on_join_ = std::move(cb); }
    void SetOnAnswer(OnAnswerCallback cb) { on_answer_ = std::move(cb); }
    void SetOnOffer(OnOfferCallback cb) { on_offer_ = std::move(cb); }
    void SetOnTrickle(OnTrickleCallback cb) { on_trickle_ = std::move(cb); }
    void SetOnTrackPublished(OnTrackPublishedCallback cb) { on_track_published_ = std::move(cb); }

private:
    void OnMessage(const std::vector<uint8_t>& data);
    bool SendRequest(const hublive::SignalRequest& request);

    WebSocketTransport* transport_;

    OnJoinCallback on_join_;
    OnAnswerCallback on_answer_;
    OnOfferCallback on_offer_;
    OnTrickleCallback on_trickle_;
    OnTrackPublishedCallback on_track_published_;
};
```

- [ ] **Step 2: Implement signaling client**

`src/signaling_client.cc`:
```cpp
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
        // Keepalive pong - silently handle
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
```

- [ ] **Step 3: Test signaling with HubLive server**

Update `src/main.cc` to use SignalingClient:
```cpp
#include "config.h"
#include "jwt_token.h"
#include "websocket_transport.h"
#include "signaling_client.h"
#include <cstdio>
#include <thread>
#include <chrono>

int main(int argc, char* argv[]) {
    std::string config_path = "config.yaml";
    if (argc > 1) config_path = argv[1];

    AppConfig config = LoadConfig(config_path);

    printf("HubLive Screen Agent (C++)\n");
    printf("  Server:  %s\n", config.hublive.url.c_str());
    printf("  Room:    %s\n", config.room.name.c_str());
    printf("  Agent:   %s\n", config.agent.identity.c_str());

    std::string token = GenerateAccessToken(config);

    WebSocketTransport ws;
    ws.SetOnClose([](int code, const std::string& reason) {
        printf("  WS closed: %s\n", reason.c_str());
    });
    ws.SetOnError([](const std::string& error) {
        printf("  WS error: %s\n", error.c_str());
    });

    SignalingClient signaling(&ws);
    bool joined = false;

    signaling.SetOnJoin([&](const hublive::JoinResponse& join) {
        printf("  Joined room: %s (server=%s)\n",
               join.room().name().c_str(),
               join.server_version().c_str());
        joined = true;
    });

    printf("  Connecting...\n");
    if (!ws.Connect(config.hublive.url, token)) {
        printf("  Failed to connect.\n");
        return 1;
    }

    // Wait for JoinResponse
    for (int i = 0; i < 50 && !joined; i++) {
        std::this_thread::sleep_for(std::chrono::milliseconds(100));
    }

    if (joined) {
        printf("  SUCCESS: Received JoinResponse\n");
    } else {
        printf("  TIMEOUT: No JoinResponse received\n");
    }

    ws.Close();
    return joined ? 0 : 1;
}
```

- [ ] **Step 4: Build and test**

```bash
cmake --build build --config Release
build\Release\screen_agent.exe
```

Expected: "Joined room: screen-share" and "SUCCESS: Received JoinResponse".

- [ ] **Step 5: Commit**

```bash
git add CLIENT/src/signaling_client.h CLIENT/src/signaling_client.cc CLIENT/src/main.cc
git commit -m "feat(client): HubLive signaling client with protobuf encode/decode"
```

---

## Task 6: Screen Capture Source

**Files:**
- Create: `CLIENT/src/screen_capture_source.h`
- Create: `CLIENT/src/screen_capture_source.cc`

Wraps libwebrtc's `DesktopCapturer` into a `VideoTrackSourceInterface` that can be added to a PeerConnection. Captures the screen at a configurable FPS, converts BGRA frames to I420 via libyuv.

- [ ] **Step 1: Define ScreenCaptureSource class**

`src/screen_capture_source.h`:
```cpp
#pragma once

#include "media/base/adapted_video_track_source.h"
#include "modules/desktop_capture/desktop_capturer.h"
#include "modules/desktop_capture/desktop_capture_options.h"
#include "rtc_base/thread.h"
#include "api/scoped_refptr.h"

#include <atomic>
#include <memory>

class ScreenCaptureSource : public rtc::AdaptedVideoTrackSource,
                            public webrtc::DesktopCapturer::Callback {
public:
    static rtc::scoped_refptr<ScreenCaptureSource> Create(int monitor_index, int fps);

    ~ScreenCaptureSource() override;

    void Start();
    void Stop();

    // VideoTrackSourceInterface
    bool is_screencast() const override { return true; }
    absl::optional<bool> needs_denoising() const override { return false; }
    SourceState state() const override { return kLive; }
    bool remote() const override { return false; }

    int width() const { return width_; }
    int height() const { return height_; }

protected:
    ScreenCaptureSource(int monitor_index, int fps);

private:
    // DesktopCapturer::Callback
    void OnCaptureResult(webrtc::DesktopCapturer::Result result,
                         std::unique_ptr<webrtc::DesktopFrame> frame) override;

    void CaptureThread();

    std::unique_ptr<webrtc::DesktopCapturer> capturer_;
    int monitor_index_;
    int fps_;
    int width_ = 0;
    int height_ = 0;
    std::atomic<bool> running_{false};
    std::unique_ptr<std::thread> capture_thread_;
};
```

- [ ] **Step 2: Implement ScreenCaptureSource**

`src/screen_capture_source.cc`:
```cpp
#include "screen_capture_source.h"

#include "api/video/i420_buffer.h"
#include "api/video/video_frame.h"
#include "third_party/libyuv/include/libyuv.h"
#include "rtc_base/time_utils.h"
#include "modules/desktop_capture/desktop_capture_types.h"

#include <cstdio>
#include <thread>
#include <chrono>

rtc::scoped_refptr<ScreenCaptureSource> ScreenCaptureSource::Create(int monitor_index, int fps) {
    return rtc::make_ref_counted<ScreenCaptureSource>(monitor_index, fps);
}

ScreenCaptureSource::ScreenCaptureSource(int monitor_index, int fps)
    : monitor_index_(monitor_index), fps_(fps) {
    auto options = webrtc::DesktopCaptureOptions::CreateDefault();
    options.set_allow_directx_capturer(true);

    capturer_ = webrtc::DesktopCapturer::CreateScreenCapturer(options);
    if (!capturer_) {
        printf("  [capture] ERROR: Failed to create screen capturer\n");
        return;
    }

    // Select the monitor
    webrtc::DesktopCapturer::SourceList sources;
    if (capturer_->GetSourceList(&sources) && monitor_index < static_cast<int>(sources.size())) {
        capturer_->SelectSource(sources[monitor_index].id);
        printf("  [capture] Selected monitor %d: %s\n", monitor_index,
               sources[monitor_index].title.c_str());
    } else if (!sources.empty()) {
        capturer_->SelectSource(sources[0].id);
        printf("  [capture] Fallback to monitor 0\n");
    }
}

ScreenCaptureSource::~ScreenCaptureSource() {
    Stop();
}

void ScreenCaptureSource::Start() {
    if (!capturer_ || running_) return;

    capturer_->Start(this);
    running_ = true;
    capture_thread_ = std::make_unique<std::thread>(&ScreenCaptureSource::CaptureThread, this);
    printf("  [capture] Started @ %d fps\n", fps_);
}

void ScreenCaptureSource::Stop() {
    running_ = false;
    if (capture_thread_ && capture_thread_->joinable()) {
        capture_thread_->join();
    }
    capture_thread_.reset();
    printf("  [capture] Stopped\n");
}

void ScreenCaptureSource::CaptureThread() {
    auto interval = std::chrono::milliseconds(1000 / fps_);

    while (running_) {
        auto start = std::chrono::steady_clock::now();
        capturer_->CaptureFrame();
        auto elapsed = std::chrono::steady_clock::now() - start;
        auto sleep_time = interval - elapsed;
        if (sleep_time > std::chrono::milliseconds(0)) {
            std::this_thread::sleep_for(sleep_time);
        }
    }
}

void ScreenCaptureSource::OnCaptureResult(
    webrtc::DesktopCapturer::Result result,
    std::unique_ptr<webrtc::DesktopFrame> frame) {

    if (result != webrtc::DesktopCapturer::Result::SUCCESS || !frame) {
        return;
    }

    int width = frame->size().width();
    int height = frame->size().height();
    width_ = width;
    height_ = height;

    // Convert BGRA -> I420
    auto i420_buffer = webrtc::I420Buffer::Create(width, height);

    libyuv::ARGBToI420(
        frame->data(), frame->stride(),
        i420_buffer->MutableDataY(), i420_buffer->StrideY(),
        i420_buffer->MutableDataU(), i420_buffer->StrideU(),
        i420_buffer->MutableDataV(), i420_buffer->StrideV(),
        width, height);

    webrtc::VideoFrame video_frame =
        webrtc::VideoFrame::Builder()
            .set_video_frame_buffer(i420_buffer)
            .set_timestamp_us(rtc::TimeMicros())
            .set_rotation(webrtc::kVideoRotation_0)
            .build();

    OnFrame(video_frame);
}
```

- [ ] **Step 3: Test screen capture standalone**

Update `src/main.cc` temporarily to verify screen capture produces frames:
```cpp
// Add after the signaling test code:
#include "screen_capture_source.h"

// In main(), after successful join:
    auto screen_source = ScreenCaptureSource::Create(config.capture.monitor, config.capture.fps);
    screen_source->Start();

    // Capture a few frames
    std::this_thread::sleep_for(std::chrono::seconds(2));
    printf("  Screen: %dx%d\n", screen_source->width(), screen_source->height());

    screen_source->Stop();
```

- [ ] **Step 4: Build and test**

```bash
cmake --build build --config Release
build\Release\screen_agent.exe
```

Expected: "Started @ 15 fps" and "Screen: 1920x1080" (or whatever the monitor resolution is).

- [ ] **Step 5: Commit**

```bash
git add CLIENT/src/screen_capture_source.h CLIENT/src/screen_capture_source.cc
git commit -m "feat(client): screen capture source using libwebrtc DesktopCapturer"
```

---

## Task 7: PeerConnection Agent

**Files:**
- Create: `CLIENT/src/peer_connection_agent.h`
- Create: `CLIENT/src/peer_connection_agent.cc`

The core orchestrator: creates PeerConnectionFactory, PeerConnection, adds the screen video track, handles SDP offer/answer exchange and ICE candidate trickle through the signaling client.

- [ ] **Step 1: Define PeerConnectionAgent class**

`src/peer_connection_agent.h`:
```cpp
#pragma once

#include "signaling_client.h"
#include "screen_capture_source.h"
#include "config.h"

#include "api/peer_connection_interface.h"
#include "api/create_peerconnection_factory.h"
#include "api/audio_codecs/builtin_audio_encoder_factory.h"
#include "api/audio_codecs/builtin_audio_decoder_factory.h"
#include "api/video_codecs/builtin_video_encoder_factory.h"
#include "api/video_codecs/builtin_video_decoder_factory.h"
#include "api/scoped_refptr.h"
#include "api/jsep.h"
#include "rtc_base/thread.h"

#include <string>
#include <memory>
#include <atomic>
#include <condition_variable>
#include <mutex>

class PeerConnectionAgent
    : public webrtc::PeerConnectionObserver,
      public webrtc::CreateSessionDescriptionObserver {
public:
    PeerConnectionAgent(SignalingClient* signaling, const AppConfig& config);
    ~PeerConnectionAgent() override;

    bool Initialize();
    void Shutdown();

    bool IsConnected() const { return ice_connected_.load(); }

private:
    // PeerConnectionObserver
    void OnSignalingChange(webrtc::PeerConnectionInterface::SignalingState new_state) override;
    void OnDataChannel(rtc::scoped_refptr<webrtc::DataChannelInterface> channel) override {}
    void OnIceGatheringChange(webrtc::PeerConnectionInterface::IceGatheringState new_state) override;
    void OnIceCandidate(const webrtc::IceCandidateInterface* candidate) override;
    void OnIceConnectionChange(webrtc::PeerConnectionInterface::IceConnectionState new_state) override;
    void OnConnectionChange(webrtc::PeerConnectionInterface::PeerConnectionState new_state) override;
    void OnTrack(rtc::scoped_refptr<webrtc::RtpTransceiverInterface> transceiver) override {}
    void OnRemoveTrack(rtc::scoped_refptr<webrtc::RtpReceiverInterface> receiver) override {}

    // CreateSessionDescriptionObserver
    void OnSuccess(webrtc::SessionDescriptionInterface* desc) override;
    void OnFailure(webrtc::RTCError error) override;

    // Internal
    void OnJoinResponse(const hublive::JoinResponse& join);
    void OnRemoteAnswer(const std::string& sdp);
    void OnRemoteOffer(const std::string& sdp);
    void OnRemoteTrickle(const std::string& candidate_json, int target);

    bool CreatePeerConnection(const hublive::JoinResponse& join);
    void AddScreenTrack();
    void CreateOffer();

    SignalingClient* signaling_;
    AppConfig config_;

    std::unique_ptr<rtc::Thread> signaling_thread_;
    std::unique_ptr<rtc::Thread> worker_thread_;
    rtc::scoped_refptr<webrtc::PeerConnectionFactoryInterface> pc_factory_;
    rtc::scoped_refptr<webrtc::PeerConnectionInterface> peer_connection_;
    rtc::scoped_refptr<ScreenCaptureSource> screen_source_;

    std::string track_cid_ = "screen_0";
    std::atomic<bool> ice_connected_{false};
};
```

- [ ] **Step 2: Implement PeerConnectionAgent**

`src/peer_connection_agent.cc`:
```cpp
#include "peer_connection_agent.h"

#include "api/peer_connection_interface.h"
#include "api/create_peerconnection_factory.h"
#include "api/audio_codecs/builtin_audio_encoder_factory.h"
#include "api/audio_codecs/builtin_audio_decoder_factory.h"
#include "api/video_codecs/builtin_video_encoder_factory.h"
#include "api/video_codecs/builtin_video_decoder_factory.h"
#include "api/enable_media.h"
#include "api/environment/environment_factory.h"
#include "api/jsep_ice_candidate.h"
#include "pc/session_description.h"

#include <cstdio>
#include <sstream>

PeerConnectionAgent::PeerConnectionAgent(SignalingClient* signaling, const AppConfig& config)
    : signaling_(signaling), config_(config) {}

PeerConnectionAgent::~PeerConnectionAgent() {
    Shutdown();
}

bool PeerConnectionAgent::Initialize() {
    // Create threads
    signaling_thread_ = rtc::Thread::Create();
    signaling_thread_->SetName("signaling", nullptr);
    signaling_thread_->Start();

    worker_thread_ = rtc::Thread::Create();
    worker_thread_->SetName("worker", nullptr);
    worker_thread_->Start();

    // Create PeerConnectionFactory
    webrtc::PeerConnectionFactoryDependencies deps;
    deps.signaling_thread = signaling_thread_.get();
    deps.worker_thread = worker_thread_.get();
    deps.audio_encoder_factory = webrtc::CreateBuiltinAudioEncoderFactory();
    deps.audio_decoder_factory = webrtc::CreateBuiltinAudioDecoderFactory();
    deps.video_encoder_factory = webrtc::CreateBuiltinVideoEncoderFactory();
    deps.video_decoder_factory = webrtc::CreateBuiltinVideoDecoderFactory();
    webrtc::EnableMedia(deps);

    pc_factory_ = webrtc::CreateModularPeerConnectionFactory(std::move(deps));
    if (!pc_factory_) {
        printf("  [pc] ERROR: Failed to create PeerConnectionFactory\n");
        return false;
    }
    printf("  [pc] PeerConnectionFactory created\n");

    // Create screen capture source
    screen_source_ = ScreenCaptureSource::Create(config_.capture.monitor, config_.capture.fps);

    // Register signaling callbacks
    signaling_->SetOnJoin([this](const hublive::JoinResponse& join) {
        OnJoinResponse(join);
    });
    signaling_->SetOnAnswer([this](const std::string& sdp) {
        OnRemoteAnswer(sdp);
    });
    signaling_->SetOnOffer([this](const std::string& sdp) {
        OnRemoteOffer(sdp);
    });
    signaling_->SetOnTrickle([this](const std::string& candidate_json, int target) {
        OnRemoteTrickle(candidate_json, target);
    });

    return true;
}

void PeerConnectionAgent::Shutdown() {
    if (screen_source_) {
        screen_source_->Stop();
    }

    if (peer_connection_) {
        peer_connection_->Close();
        peer_connection_ = nullptr;
    }

    pc_factory_ = nullptr;

    if (signaling_thread_) {
        signaling_thread_->Stop();
    }
    if (worker_thread_) {
        worker_thread_->Stop();
    }
}

void PeerConnectionAgent::OnJoinResponse(const hublive::JoinResponse& join) {
    printf("  [pc] Joined room '%s'\n", join.room().name().c_str());

    if (!CreatePeerConnection(join)) {
        printf("  [pc] ERROR: Failed to create PeerConnection\n");
        return;
    }

    AddScreenTrack();
    CreateOffer();
}

bool PeerConnectionAgent::CreatePeerConnection(const hublive::JoinResponse& join) {
    webrtc::PeerConnectionInterface::RTCConfiguration rtc_config;
    rtc_config.sdp_semantics = webrtc::SdpSemantics::kUnifiedPlan;

    // Add ICE servers from JoinResponse
    for (const auto& ice_server : join.ice_servers()) {
        webrtc::PeerConnectionInterface::IceServer server;
        for (const auto& url : ice_server.urls()) {
            server.urls.push_back(url);
        }
        server.username = ice_server.username();
        server.password = ice_server.credential();
        rtc_config.servers.push_back(server);
    }

    // If no ICE servers provided, add a default STUN server
    if (rtc_config.servers.empty()) {
        webrtc::PeerConnectionInterface::IceServer stun;
        stun.urls.push_back("stun:stun.l.google.com:19302");
        rtc_config.servers.push_back(stun);
    }

    webrtc::PeerConnectionDependencies pc_deps(this);
    auto result = pc_factory_->CreatePeerConnectionOrError(rtc_config, std::move(pc_deps));
    if (!result.ok()) {
        printf("  [pc] CreatePeerConnection failed: %s\n", result.error().message());
        return false;
    }

    peer_connection_ = result.MoveValue();
    printf("  [pc] PeerConnection created\n");
    return true;
}

void PeerConnectionAgent::AddScreenTrack() {
    // Start screen capture
    screen_source_->Start();

    // Wait briefly for first frame to get dimensions
    std::this_thread::sleep_for(std::chrono::milliseconds(200));

    // Send AddTrack request to server BEFORE creating SDP offer
    signaling_->SendAddTrack(
        track_cid_, "screen",
        hublive::TrackType::VIDEO,
        hublive::TrackSource::SCREEN_SHARE,
        screen_source_->width(), screen_source_->height());

    // Create video track and add to PeerConnection
    auto video_track = pc_factory_->CreateVideoTrack(screen_source_, "screen_share");
    if (!video_track) {
        printf("  [pc] ERROR: Failed to create video track\n");
        return;
    }

    auto result = peer_connection_->AddTrack(video_track, {"stream_0"});
    if (!result.ok()) {
        printf("  [pc] ERROR: AddTrack failed: %s\n", result.error().message());
        return;
    }

    printf("  [pc] Video track added (%dx%d)\n",
           screen_source_->width(), screen_source_->height());
}

void PeerConnectionAgent::CreateOffer() {
    webrtc::PeerConnectionInterface::RTCOfferAnswerOptions options;
    options.offer_to_receive_video = 0;
    options.offer_to_receive_audio = 0;

    peer_connection_->CreateOffer(
        rtc::scoped_refptr<webrtc::CreateSessionDescriptionObserver>(this),
        options);
}

// CreateSessionDescriptionObserver callbacks

void PeerConnectionAgent::OnSuccess(webrtc::SessionDescriptionInterface* desc) {
    std::string sdp;
    desc->ToString(&sdp);
    printf("  [pc] Local SDP created (type=%s, %zu bytes)\n",
           desc->type().c_str(), sdp.size());

    peer_connection_->SetLocalDescription(
        std::unique_ptr<webrtc::SessionDescriptionInterface>(desc->Clone()));

    signaling_->SendOffer(sdp);
}

void PeerConnectionAgent::OnFailure(webrtc::RTCError error) {
    printf("  [pc] CreateOffer failed: %s\n", error.message());
}

// Signaling callbacks

void PeerConnectionAgent::OnRemoteAnswer(const std::string& sdp) {
    webrtc::SdpParseError error;
    auto answer = webrtc::CreateSessionDescription(
        webrtc::SdpType::kAnswer, sdp, &error);
    if (!answer) {
        printf("  [pc] Failed to parse answer SDP: %s\n", error.description.c_str());
        return;
    }

    peer_connection_->SetRemoteDescription(std::move(answer));
    printf("  [pc] Remote answer set\n");
}

void PeerConnectionAgent::OnRemoteOffer(const std::string& sdp) {
    // Server may send an offer for subscriber transport
    webrtc::SdpParseError error;
    auto offer = webrtc::CreateSessionDescription(
        webrtc::SdpType::kOffer, sdp, &error);
    if (!offer) {
        printf("  [pc] Failed to parse offer SDP: %s\n", error.description.c_str());
        return;
    }

    peer_connection_->SetRemoteDescription(std::move(offer));

    // Create answer for subscriber
    webrtc::PeerConnectionInterface::RTCOfferAnswerOptions opts;
    peer_connection_->CreateAnswer(
        rtc::scoped_refptr<webrtc::CreateSessionDescriptionObserver>(this),
        opts);
}

void PeerConnectionAgent::OnRemoteTrickle(const std::string& candidate_json, int target) {
    // Parse ICE candidate from JSON
    // The candidate_json is a JSON object with "candidate", "sdpMid", "sdpMLineIndex"
    // Simple parsing since we know the format
    std::string candidate_str, sdp_mid;
    int sdp_mline_index = 0;

    // Extract "candidate" field
    auto extract_string = [&](const std::string& json, const std::string& key) -> std::string {
        auto pos = json.find("\"" + key + "\"");
        if (pos == std::string::npos) return "";
        pos = json.find("\"", pos + key.size() + 2);
        if (pos == std::string::npos) return "";
        pos++;
        auto end = json.find("\"", pos);
        if (end == std::string::npos) return "";
        return json.substr(pos, end - pos);
    };

    auto extract_int = [&](const std::string& json, const std::string& key) -> int {
        auto pos = json.find("\"" + key + "\"");
        if (pos == std::string::npos) return 0;
        pos = json.find(":", pos);
        if (pos == std::string::npos) return 0;
        pos++;
        while (pos < json.size() && json[pos] == ' ') pos++;
        return std::stoi(json.substr(pos));
    };

    candidate_str = extract_string(candidate_json, "candidate");
    sdp_mid = extract_string(candidate_json, "sdpMid");
    sdp_mline_index = extract_int(candidate_json, "sdpMLineIndex");

    if (candidate_str.empty()) return;

    webrtc::SdpParseError error;
    auto ice_candidate = webrtc::CreateIceCandidate(sdp_mid, sdp_mline_index, candidate_str, &error);
    if (!ice_candidate) {
        printf("  [pc] Failed to parse ICE candidate: %s\n", error.description.c_str());
        return;
    }

    peer_connection_->AddIceCandidate(ice_candidate.get());
}

// PeerConnectionObserver

void PeerConnectionAgent::OnSignalingChange(
    webrtc::PeerConnectionInterface::SignalingState new_state) {
    printf("  [pc] Signaling state: %d\n", static_cast<int>(new_state));
}

void PeerConnectionAgent::OnIceGatheringChange(
    webrtc::PeerConnectionInterface::IceGatheringState new_state) {
    printf("  [pc] ICE gathering state: %d\n", static_cast<int>(new_state));
}

void PeerConnectionAgent::OnIceCandidate(const webrtc::IceCandidateInterface* candidate) {
    std::string candidate_str;
    candidate->ToString(&candidate_str);

    // Format as JSON for HubLive signaling
    std::ostringstream json;
    json << "{"
         << "\"candidate\":\"" << candidate_str << "\","
         << "\"sdpMid\":\"" << candidate->sdp_mid() << "\","
         << "\"sdpMLineIndex\":" << candidate->sdp_mline_index()
         << "}";

    signaling_->SendTrickle(json.str(), 0);  // 0 = PUBLISHER target
}

void PeerConnectionAgent::OnIceConnectionChange(
    webrtc::PeerConnectionInterface::IceConnectionState new_state) {
    printf("  [pc] ICE connection state: %d\n", static_cast<int>(new_state));
}

void PeerConnectionAgent::OnConnectionChange(
    webrtc::PeerConnectionInterface::PeerConnectionState new_state) {
    const char* state_str = "unknown";
    switch (new_state) {
        case webrtc::PeerConnectionInterface::PeerConnectionState::kNew: state_str = "new"; break;
        case webrtc::PeerConnectionInterface::PeerConnectionState::kConnecting: state_str = "connecting"; break;
        case webrtc::PeerConnectionInterface::PeerConnectionState::kConnected:
            state_str = "CONNECTED";
            ice_connected_ = true;
            break;
        case webrtc::PeerConnectionInterface::PeerConnectionState::kDisconnected:
            state_str = "disconnected";
            ice_connected_ = false;
            break;
        case webrtc::PeerConnectionInterface::PeerConnectionState::kFailed:
            state_str = "FAILED";
            ice_connected_ = false;
            break;
        case webrtc::PeerConnectionInterface::PeerConnectionState::kClosed:
            state_str = "closed";
            ice_connected_ = false;
            break;
    }
    printf("  [pc] Connection state: %s\n", state_str);
}
```

- [ ] **Step 3: Build and verify compilation**

```bash
cmake --build build --config Release
```

Expected: Compiles without errors. (Full integration test is in Task 8.)

- [ ] **Step 4: Commit**

```bash
git add CLIENT/src/peer_connection_agent.h CLIENT/src/peer_connection_agent.cc
git commit -m "feat(client): PeerConnection agent with SDP/ICE negotiation"
```

---

## Task 8: Full Integration - main.cc

**Files:**
- Modify: `CLIENT/src/main.cc`

Wire everything together: config -> JWT -> WebSocket -> signaling -> PeerConnection -> screen capture -> stream to HubLive.

- [ ] **Step 1: Write the final main.cc**

`src/main.cc`:
```cpp
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
    printf("  Connecting to %s...\n", config.hublive.url.c_str());
    if (!ws.Connect(config.hublive.url, token)) {
        printf("  ERROR: WebSocket connection failed\n");
        return 1;
    }
    printf("  WebSocket connected\n\n");

    // 5. Handle Ctrl+C
    std::signal(SIGINT, SignalHandler);
    std::signal(SIGTERM, SignalHandler);

    // 6. Main loop - keepalive ping + status
    printf("  Streaming... (Ctrl+C to stop)\n\n");
    int ping_counter = 0;
    while (g_running) {
        std::this_thread::sleep_for(std::chrono::seconds(1));

        // Send ping every 10 seconds
        if (++ping_counter >= 10) {
            signaling.SendPing();
            ping_counter = 0;
        }

        // Check connection health
        if (!ws.IsConnected()) {
            printf("  WebSocket disconnected, shutting down.\n");
            break;
        }
    }

    // 7. Cleanup
    printf("\n  Stopping...\n");
    agent.Shutdown();
    ws.Close();
    printf("  Agent stopped.\n");

    return 0;
}
```

- [ ] **Step 2: Build final executable**

```bash
cd C:\Users\Admin\Desktop\WCAP\DEVBYHOON\CLIENT
cmake --build build --config Release
```

Expected: `build\Release\screen_agent.exe` builds successfully.

- [ ] **Step 3: End-to-end test**

Prerequisites:
1. HubLive server running: `hublive-server --dev` (port 7880)
2. Web dashboard running: `cd WEB && npm run dev`

Run the agent:
```bash
cd build\Release
copy ..\..\config.yaml .
screen_agent.exe
```

Expected output:
```
=== HubLive Screen Agent (C++) ===
  Server:  ws://localhost:7880
  Room:    screen-share
  Agent:   agent-cpp-001 (Screen Agent C++)
  Capture: monitor=0 fps=15 scale=1.0

  Token generated
  Connecting to ws://localhost:7880...
  WebSocket connected

  [signal] JoinResponse: room=screen-share participant=agent-cpp-001
  [pc] Joined room 'screen-share'
  [pc] PeerConnection created
  [capture] Selected monitor 0: ...
  [capture] Started @ 15 fps
  [signal] SendAddTrack: cid=screen_0 name=screen 1920x1080
  [pc] Video track added (1920x1080)
  [pc] Local SDP created (type=offer, XXXX bytes)
  [signal] Sending offer (XXXX bytes SDP)
  [signal] TrackPublished: cid=screen_0 sid=TR_...
  [signal] Answer received
  [pc] Remote answer set
  [pc] ICE gathering state: 1
  [pc] Connection state: connecting
  [pc] Connection state: CONNECTED
  Streaming... (Ctrl+C to stop)
```

Then open the web dashboard and verify the screen stream is visible.

- [ ] **Step 4: Verify on web viewer**

Open the web dashboard at the configured URL. Navigate to the dashboard page. The agent's screen should appear as a live video stream in the computer grid.

- [ ] **Step 5: Commit**

```bash
git add CLIENT/src/main.cc
git commit -m "feat(client): complete C++ screen agent with end-to-end streaming"
```

---

## Task 9: Build Script & Distribution

**Files:**
- Create: `CLIENT/build.bat`

Create a simple build script for convenience.

- [ ] **Step 1: Create build.bat**

`CLIENT/build.bat`:
```batch
@echo off
echo === Building HubLive Screen Agent ===
echo.

if not exist build (
    echo Configuring CMake...
    cmake -B build -G "Visual Studio 17 2022" -A x64
    if errorlevel 1 (
        echo CMake configure failed!
        exit /b 1
    )
)

echo Building Release...
cmake --build build --config Release
if errorlevel 1 (
    echo Build failed!
    exit /b 1
)

echo.
echo === Build complete ===
echo Output: build\Release\screen_agent.exe
echo.
echo To run: copy config.yaml to build\Release\ and run screen_agent.exe
```

- [ ] **Step 2: Test build script**

```bash
cd C:\Users\Admin\Desktop\WCAP\DEVBYHOON\CLIENT
build.bat
```

Expected: Builds successfully, prints output path.

- [ ] **Step 3: Commit**

```bash
git add CLIENT/build.bat
git commit -m "feat(client): add build script for convenience"
```

---

## Summary of Components

| Component | File | Responsibility |
|-----------|------|---------------|
| Config | `config.h/cc` | Parse config.yaml |
| JWT | `jwt_token.h/cc` | Generate HubLive access token (BoringSSL HMAC-SHA256) |
| WebSocket | `websocket_transport.h/cc` | WinHTTP WebSocket client |
| Signaling | `signaling_client.h/cc` | HubLive protobuf encode/decode |
| Screen Capture | `screen_capture_source.h/cc` | DesktopCapturer -> VideoTrackSource |
| PeerConnection | `peer_connection_agent.h/cc` | WebRTC SDP/ICE orchestration |
| Main | `main.cc` | Entry point, lifecycle, Ctrl+C handling |
| Proto | `proto/*.proto` | HubLive protocol definitions |

## Dependencies

All from libwebrtc build - **zero external dependencies**:
- libwebrtc (PeerConnection, DesktopCapturer, codecs)
- BoringSSL (JWT HMAC-SHA256)
- protobuf (HubLive signaling messages)
- libyuv (BGRA -> I420 frame conversion)
- WinHTTP (WebSocket, built into Windows)

## Data Flow

```
Screen → DesktopCapturer → BGRA → libyuv → I420 → VideoTrackSource
    → PeerConnection → RTP → WebSocket/ICE → HubLive Server → Web Viewer
```

## Signaling Flow

```
Agent                          HubLive Server
  |-- WebSocket + JWT -------->|
  |<-- JoinResponse -----------|
  |-- AddTrackRequest -------->|
  |<-- TrackPublished ---------|
  |-- SDP Offer -------------->|
  |<-- SDP Answer -------------|
  |<=> ICE Trickle <===========>|
  |=== RTP Media =============>| --> Web Viewer
  |--- Ping ------------------->|
  |<-- Pong -------------------|
```
