# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

HubLive (WCAP) is a remote screen streaming platform using WebRTC. Three independent components communicate via WebRTC + WebSocket/HTTP protocols:

```
CLIENT (C++ Agent, Windows) --WebRTC/HubLive WS--> SERVER (Go SFU) --WebRTC--> WEB (React Viewer)
```

- **CLIENT/**: C++ screen capture agent using libwebrtc (Windows only)
- **SERVER/**: Go HubLive SFU server (forked/customized HubLive)
- **WEB/**: Web dashboard viewer (React + TypeScript, in early development)

## Build & Run Commands

### CLIENT (C++ Screen Agent)

```bash
cd CLIENT
build.bat                           # CMake configure + Ninja build
# Output: build/screen_agent.exe

# Manual CMake (uses clang-cl from libwebrtc build tree):
cmake -B build -G Ninja \
  -DCMAKE_C_COMPILER="f:/webrtc-checkout/src/third_party/llvm-build/Release+Asserts/bin/clang-cl.exe" \
  -DCMAKE_CXX_COMPILER="f:/webrtc-checkout/src/third_party/llvm-build/Release+Asserts/bin/clang-cl.exe" \
  -DCMAKE_LINKER_TYPE=LLD
cmake --build build

# Run (config.yaml must be in working directory or passed as arg):
copy config.yaml build\
build\screen_agent.exe [path/to/config.yaml]
```

Requires: libwebrtc pre-built at `f:\webrtc-checkout\src\out\Release\`, CMake >= 3.20, Ninja.

### SERVER (Go HubLive SFU)

```bash
cd SERVER

# Build with Mage (preferred):
mage build                          # Output: bin/hublive-server
mage deps                           # Install build tools (Wire)
mage generate                       # Regenerate Wire DI + go generate

# Alternative direct Go build:
go build -o bin/hublive-server ./cmd/server

# Run in dev mode (API Key: devkey, Secret: secret):
bin/hublive-server --dev

# Tests:
mage test                           # Unit tests only (-short)
mage testAll                        # All tests including integration (timeout 4m)
go test -short ./... -count=1       # Direct unit tests
go test ./... -count=1 -timeout=4m  # Direct all tests
```

Requires: Go 1.25+, Mage build tool.

### WEB (Dashboard Viewer)

Currently a standalone `viewer.html` served via any HTTP server:
```bash
cd WEB
python -m http.server 8080          # Then open http://localhost:8080/viewer.html
```

Planned stack: React 19 + TypeScript + Vite (port 5173), Zustand, TanStack React Query, shadcn/ui + Tailwind CSS v4.

## Architecture

### CLIENT — C++ Screen Agent

Signaling flow: Load config.yaml -> Generate JWT (HS256/BoringSSL) -> Try HubLive WebSocket + protobuf -> Fallback to WHIP HTTP -> Screen capture loop (BGRA -> I420 -> RTP) -> Ctrl+C graceful shutdown.

Key files in `src/`: `main.cc` (entry), `config.h/cc` (YAML parser), `jwt_token.h/cc` (JWT), `websocket_transport.h/cc` (WinHTTP WS), `signaling_client.h/cc` (HubLive protobuf), `whip_client.h/cc` (HTTP fallback), `screen_capture_source.h/cc` (capture), `peer_connection_agent.h/cc` (PeerConnection/SDP/ICE).

All dependencies come from the libwebrtc build tree (BoringSSL, protobuf, libyuv) plus Windows APIs (WinHTTP, DXGI, Direct3D 11). Must use clang-cl from libwebrtc for ABI compatibility.

### SERVER — Go HubLive SFU

Layered architecture (see `pkg/ARCHITECTURE.md` for full details):
- **Layer 0** (Foundation): `utils/`, `config/`, `metric/`
- **Layer 1** (Core): `sfu/` (media engine — do not modify), `domain/` (extension interfaces)
- **Layer 2** (Infra): `store/` (local/redis), `transport/`, `routing/`, `rtc/`
- **Layer 2.5** (App): `service/roommanager`, `service/roomallocator`, `service/ioservice`
- **Layer 3** (API): `api/middleware/` (auth), Twirp RPC handlers in `service/`
- **Layer 4** (Bootstrap): `service/server`, `service/wire` (Google Wire DI), `service/turn`

Rule: Layer N imports Layer 0..N-1 only. Entry point: `cmd/server/main.go`.

Extension points: Transport protocols (`transport.Transport`), Storage backends (`store.ObjectStore`), Subscription policies, Agent dispatch/worker registry.

DI uses Google Wire — run `mage generate` after changing `service/wire.go`.

### WEB — Dashboard Viewer

Currently minimal (single HTML file using HubLive JS SDK from CDN). Full React app planned with:
- Zustand for client state, React Query for server state
- shadcn/ui component library
- HubLive client SDK for stream subscription
- JWT auth with Axios interceptors

## Dev Environment

- **Platform**: Windows 10+ x64
- **Dev server credentials**: API Key `devkey`, API Secret `secret` (used with `--dev` flag)
- **Default HubLive URL**: `ws://localhost:7880`
- **Default room**: `screen-share`

## Key Constraints

- CLIENT must be compiled with clang-cl from the libwebrtc build tree (`f:\webrtc-checkout\`) — MSVC ABI is incompatible
- CLIENT disables RTTI (`/GR-`) and uses libc++ to match libwebrtc's build configuration
- SERVER's `pkg/sfu/` is the core media engine — avoid modifying it
- SERVER uses Wire for DI — edit `service/wire.go` and run `mage generate`, never edit `wire_gen.go` directly
- Protobuf files in `CLIENT/proto/` define the HubLive signaling protocol — regeneration requires protoc
