# HubLive - Remote Screen Streaming Platform

Hệ thống stream màn hình từ xa qua WebRTC, gồm 3 thành phần chính.

## Kiến trúc

```
┌─────────────┐     WebRTC/RTP      ┌─────────────┐     WebRTC/RTP      ┌─────────────┐
│   CLIENT    │ ──────────────────► │   SERVER    │ ──────────────────► │    WEB      │
│  C++ Agent  │   HubLive WS hoặc   │  HubLive SFU │   Browser WebRTC   │   Viewer    │
│  (Windows)  │   WHIP fallback     │   (Go)      │                    │  (React/JS) │
└─────────────┘                     └─────────────┘                     └─────────────┘
```

## Thành phần

| Thư mục | Mô tả | Ngôn ngữ |
|---------|--------|----------|
| [CLIENT/](CLIENT/) | Agent capture màn hình + stream | C++ (libwebrtc) |
| [SERVER/](SERVER/) | HubLive SFU server | Go |
| [WEB/](WEB/) | Dashboard xem stream | React + TypeScript |

## Quick Start

```bash
# 1. Start server
cd SERVER/bin
hublive-server.exe --dev

# 2. Build & run agent
cd CLIENT
build.bat
copy config.yaml build\
build\screen_agent.exe

# 3. Xem stream
# Mở WEB/viewer.html qua HTTP server
cd WEB
python -m http.server 8080
# Truy cập http://localhost:8080/viewer.html
```

## Trạng thái

- [x] Video stream (màn hình + cursor)
- [x] HubLive WebSocket signaling
- [x] WHIP HTTP fallback
- [x] Web viewer (test)
- [ ] Audio stream
- [ ] Remote mouse/keyboard control
- [ ] Multi-monitor
- [ ] Auto-reconnect
