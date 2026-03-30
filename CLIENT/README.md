# CLIENT - C++ Screen Streaming Agent

Agent chạy trên Windows, capture màn hình (có cursor) và stream tới HubLive server qua WebRTC.

## Tính năng

- Capture màn hình bằng libwebrtc DesktopCapturer (DirectX/GDI)
- Hiển thị cursor trong stream (DesktopAndCursorComposer)
- Signaling: HubLive WebSocket + protobuf (primary)
- Signaling: WHIP HTTP POST (fallback tự động)
- JWT token authentication (BoringSSL HMAC-SHA256)
- Config từ file `config.yaml`
- Video codecs: VP8, VP9, H264, AV1

## Yêu cầu build

- **libwebrtc** đã build tại `f:\webrtc-checkout\src\out\Release\`
- **CMake** >= 3.20
- **Ninja** build system
- **clang-cl** (từ libwebrtc build, tự động dùng)
- **Windows 10+** x64

## Build

```bash
# Lần đầu
build.bat

# Hoặc manual
cmake -B build -G Ninja \
  -DCMAKE_C_COMPILER="f:/webrtc-checkout/src/third_party/llvm-build/Release+Asserts/bin/clang-cl.exe" \
  -DCMAKE_CXX_COMPILER="f:/webrtc-checkout/src/third_party/llvm-build/Release+Asserts/bin/clang-cl.exe" \
  -DCMAKE_LINKER_TYPE=LLD
cmake --build build
```

Output: `build/screen_agent.exe` (~53MB)

## Chạy

```bash
copy config.yaml build\
build\screen_agent.exe
```

Hoặc chỉ định config path:
```bash
build\screen_agent.exe path\to\config.yaml
```

## Config (config.yaml)

```yaml
hublive:
  url: "ws://localhost:7880"    # Server URL (ws:// hoặc wss://)
  api_key: "devkey"             # API key
  api_secret: "secret"          # API secret

room:
  name: "screen-share"          # Tên room

agent:
  identity: "agent-cpp-001"     # ID của agent
  name: "Screen Agent C++"      # Tên hiển thị

capture:
  fps: 15                       # Frame per second
  monitor: 1                    # Monitor (1=primary, 2=secondary)
  scale: 1.0                    # Scale factor (0.5=half)
```

## Cấu trúc code

```
src/
  main.cc                    # Entry point, lifecycle, Ctrl+C
  config.h/cc                # Load config.yaml
  jwt_token.h/cc             # JWT HS256 token (BoringSSL)
  websocket_transport.h/cc   # WinHTTP WebSocket client
  signaling_client.h/cc      # HubLive protobuf signaling
  whip_client.h/cc           # WHIP HTTP fallback
  screen_capture_source.h/cc # Screen capture + cursor -> I420
  peer_connection_agent.h/cc # PeerConnection, SDP/ICE
proto/
  hublive_models.proto       # HubLive protocol models
  hublive_rtc.proto          # HubLive signaling messages
```

## Luồng hoạt động

```
1. Load config.yaml
2. Generate JWT token
3. Try WebSocket -> HubLive signaling
   ├─ OK: JoinResponse -> AddTrack -> SDP Offer/Answer -> ICE -> Stream
   └─ FAIL: Fallback to WHIP
              POST SDP offer to /whip/v1 -> SDP answer -> ICE -> Stream
4. Screen capture loop (BGRA -> I420 -> RTP)
5. Ctrl+C -> Graceful shutdown
```

## Dependencies (all from libwebrtc build)

- libwebrtc (PeerConnection, DesktopCapturer, codecs)
- BoringSSL (JWT HMAC-SHA256)
- protobuf (HubLive signaling)
- libyuv (BGRA -> I420)
- WinHTTP (WebSocket + HTTP, built into Windows)
