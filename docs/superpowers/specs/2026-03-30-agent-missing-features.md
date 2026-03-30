# HubLive Agent — Missing Features Design Spec

**Date:** 2026-03-30
**Scope:** 4 features to complete the C++ screen agent

## Overview

The agent currently streams screen video (with cursor) via WebRTC. Four features remain:

1. Auto-reconnect (stability)
2. Audio stream — system loopback + microphone, mixed
3. Remote mouse/keyboard control — web viewer → agent via DataChannel
4. Multi-monitor — simultaneous streams from multiple displays

## 1. Auto-Reconnect

### Design

Wrap the entire connect-stream lifecycle in a retry loop with exponential backoff + jitter.

```
                    ┌──────────────────────────┐
                    │     main() retry loop     │
                    │                           │
  ┌────────────►    │  1. Generate JWT          │
  │                 │  2. Connect WebSocket      │
  │                 │  3. Join room              │
  │                 │  4. Add tracks + stream    │
  │                 │  5. Run capture loop       │
  │                 │                           │
  │                 │  On disconnect/error:      │
  │   backoff       │  - Cleanup PeerConnection  │
  │   + jitter      │  - Close WebSocket         │
  │                 │  - Calculate delay          │
  └─────────────────│  - Sleep(delay)            │
                    │  - Retry                   │
                    └──────────────────────────┘
```

### Backoff Algorithm

```cpp
int delay = std::min(30, (1 << retry_count));  // 1, 2, 4, 8, 16, 30, 30...
delay += rand() % 3;                           // +0~2s jitter
```

- Max delay: 30 seconds
- On successful connection: reset retry_count to 0
- On Ctrl+C: break out of retry loop, shutdown gracefully
- Keep ScreenCaptureSource alive across retries (avoid reinit overhead)

### Changes

| File | Change |
|------|--------|
| `main.cc` | Wrap connect+stream in retry loop |
| `peer_connection_agent.h/cc` | Add `Disconnect()` method for clean teardown |
| `signaling_client.h/cc` | Add `Reset()` to allow reconnection |
| `websocket_transport.h/cc` | Add `Close()` + reconnect support |

### Error Detection

- WebSocket `onClose` / `onError` callback
- PeerConnection state → `kDisconnected` or `kFailed`
- Ping timeout (no pong response within 15s)

---

## 2. Audio Stream

### Design

Two audio sources captured via Windows WASAPI, mixed into one PCM stream, fed into libwebrtc as a custom AudioSource.

```
WASAPI Mic (any sample rate)
    │
    ├─→ Resample to 48kHz/mono ──┐
    │                              │
    │                              ├─→ AudioMixer (sum + clip) ──→ CustomAudioSource
    │                              │                                     │
WASAPI Loopback (system audio)    │                               WebRTC Audio Track
    │                              │                                     │
    └─→ Resample to 48kHz/stereo ─┘                              Opus Encode → RTP
```

### Components

#### WasapiCapture (`audio_capture.h/cc`)

Captures audio from a WASAPI endpoint (render loopback or capture device).

```cpp
class WasapiCapture {
public:
    bool Init(bool loopback);         // true=system audio, false=mic
    bool Start();
    void Stop();

    // Callback: delivers PCM float32 frames
    using OnDataCallback = std::function<void(const float* data, int frames, int channels, int sample_rate)>;
    void SetOnData(OnDataCallback cb);

private:
    IAudioClient* audio_client_;
    IAudioCaptureClient* capture_client_;
    std::thread capture_thread_;
};
```

- Loopback: `eRender` + `AUDCLNT_STREAMFLAGS_LOOPBACK`
- Mic: `eCapture` default device
- Both deliver float32 PCM
- Capture thread polls `GetBuffer()` every 10ms

#### AudioResampler (`audio_resampler.h/cc`)

Converts between sample rates using libspeexdsp (from libwebrtc third_party).

```cpp
class AudioResampler {
public:
    bool Init(int src_rate, int src_channels, int dst_rate, int dst_channels);
    void Process(const float* in, int in_frames, float* out, int* out_frames);
private:
    SpeexResamplerState* resampler_;
};
```

Target: 48000Hz, mono (for Opus compatibility).

#### AudioMixer (`audio_mixer.h/cc`)

Mixes 2 audio streams with timestamp alignment.

```cpp
class AudioMixer {
public:
    void SetMicVolume(float gain);        // 0.0 - 1.0
    void SetSystemVolume(float gain);     // 0.0 - 1.0

    void PushMicData(const float* data, int frames);
    void PushSystemData(const float* data, int frames);

    // Returns mixed buffer (10ms frames for WebRTC = 480 samples at 48kHz)
    bool GetMixedFrame(float* out, int frames);

private:
    RingBuffer mic_buffer_;
    RingBuffer system_buffer_;
    float mic_gain_ = 1.0f;
    float system_gain_ = 0.8f;  // system slightly lower by default
};
```

- Ring buffer per source to handle async delivery
- Mix: `out[i] = clamp(mic[i] * mic_gain + system[i] * sys_gain, -1.0, 1.0)`
- 10ms frame alignment (480 samples at 48kHz)

#### CustomAudioSource

Implements `webrtc::AudioSourceInterface` to feed mixed PCM into libwebrtc.

```cpp
class CustomAudioSource : public webrtc::AudioSourceInterface {
public:
    void OnData(const float* data, int frames, int sample_rate, int channels);
    // webrtc::AudioSourceInterface methods...
};
```

### Changes

| File | Change |
|------|--------|
| NEW `audio_capture.h/cc` | WASAPI loopback + mic capture |
| NEW `audio_resampler.h/cc` | Sample rate conversion |
| NEW `audio_mixer.h/cc` | Mix 2 sources + ring buffer |
| NEW `custom_audio_source.h/cc` | Feed PCM into libwebrtc |
| `peer_connection_agent.h/cc` | Add audio track to PeerConnection |
| `signaling_client.h/cc` | `AddTrack(AUDIO, MICROPHONE)` |
| `config.h/cc` | Add `audio.system_enabled`, `audio.mic_enabled`, `audio.system_gain`, `audio.mic_gain` |
| `CMakeLists.txt` | Link ole32, uuid (COM for WASAPI) |

### Config Extension

```yaml
audio:
  system_enabled: true     # Capture system audio (loopback)
  mic_enabled: true        # Capture microphone
  system_gain: 0.8         # System audio volume (0.0-1.0)
  mic_gain: 1.0            # Mic volume (0.0-1.0)
```

---

## 3. Remote Mouse/Keyboard Control

### Design

Web viewer captures input events → sends via WebRTC DataChannel → Agent receives → injects into Windows via `SendInput()`.

```
Web Viewer                          Agent (C++)
─────────                          ──────────
mousedown/mousemove/keydown
        │
        ▼
Serialize to MessagePack   ──DataChannel──►   Deserialize MessagePack
        │                                              │
   Normalized coords                            Scale to screen coords
   (0.0 - 1.0)                                 (pixels)
                                                       │
                                                       ▼
                                               SendInput() Win32 API
```

### Protocol (MessagePack binary)

```
Mouse event:
{
  "t": 1,           // type: 1=mouse
  "s": 12345,       // sequence number
  "a": 1,           // action: 1=move, 2=down, 3=up, 4=scroll
  "x": 0.5234,      // normalized X (0.0-1.0)
  "y": 0.3421,      // normalized Y (0.0-1.0)
  "b": 0,           // button: 0=none, 1=left, 2=right, 3=middle
  "d": 0            // scroll delta (for scroll events)
}

Keyboard event:
{
  "t": 2,           // type: 2=keyboard
  "s": 12346,       // sequence number
  "a": 1,           // action: 1=keydown, 2=keyup
  "k": 65,          // key code (JS keyCode)
  "m": 5            // modifier bitmask: 1=Ctrl, 2=Shift, 4=Alt, 8=Meta
}
```

Short keys (`t`, `s`, `a`, `x`, `y`, `b`, `d`, `k`, `m`) to minimize payload size.

### Agent Side — InputInjector (`input_injector.h/cc`)

```cpp
class InputInjector {
public:
    void Init(int screen_width, int screen_height);

    void InjectMouseMove(float norm_x, float norm_y);
    void InjectMouseButton(float norm_x, float norm_y, int button, bool down);
    void InjectMouseScroll(float norm_x, float norm_y, int delta);
    void InjectKeyboard(int key_code, bool down, int modifiers);

private:
    int screen_width_;
    int screen_height_;
    uint32_t last_seq_ = 0;  // detect out-of-order

    void SendMouseInput(int x, int y, DWORD flags);
    void SendKeyInput(WORD vk, bool down);
    WORD JsKeyCodeToVirtualKey(int js_key_code);
};
```

- Normalized → absolute pixel: `pixel_x = norm_x * 65535` (for `MOUSEEVENTF_ABSOLUTE`)
- Out-of-order detection: drop if `seq < last_seq_`
- Modifier keys (Ctrl/Shift/Alt): inject key down before, key up after
- Key code mapping: JS keyCode → Win32 Virtual Key Code

### Web Side Changes (viewer.html / future React)

```javascript
// Capture events on video element
video.addEventListener('mousemove', (e) => {
    const rect = video.getBoundingClientRect();
    sendInput({
        t: 1, s: seq++, a: 1,
        x: (e.clientX - rect.left) / rect.width,
        y: (e.clientY - rect.top) / rect.height,
        b: 0, d: 0
    });
});
```

### DataChannel Setup

- Label: `"input"`
- Ordered: true
- MaxRetransmits: 0 (unreliable for mouse move — drop stale, keep fresh)
- Reliable for keyboard events (ordered + reliable sub-channel, or accept ordered delivery)

### Changes

| File | Change |
|------|--------|
| NEW `input_injector.h/cc` | Win32 SendInput wrapper |
| NEW `msgpack_lite.h` | Minimal MessagePack decoder (header-only, ~100 lines) |
| `peer_connection_agent.h/cc` | Create DataChannel, route messages to InputInjector |
| `WEB/viewer.html` | Add mouse/keyboard event capture + DataChannel send |
| `config.h/cc` | Add `control.enabled`, `control.mouse`, `control.keyboard` |

### Config Extension

```yaml
control:
  enabled: true            # Enable remote control
  mouse: true              # Accept mouse input
  keyboard: true           # Accept keyboard input
```

---

## 4. Multi-Monitor

Deferred — implement after features 1-3 are stable. Will be designed in a separate spec. Basic idea: enumerate monitors via `GetMonitorInfo()`, create one capture source + one video track per monitor, publish as separate tracks with monitor metadata.

---

## Implementation Order

| Phase | Feature | Agent | Depends on |
|-------|---------|-------|-----------|
| 1 | Auto-reconnect | Agent 1 | None |
| 2 | Audio stream | Agent 2 | None |
| 3 | Remote control | Agent 3 | None |
| 4 | Review + fix | Agent 4 | Agents 1-3 complete |

All 3 implementation agents run in parallel. Agent 4 reviews after all complete.

## File Summary (new files)

```
CLIENT/src/
  audio_capture.h/cc           # WASAPI loopback + mic
  audio_resampler.h/cc         # Sample rate conversion
  audio_mixer.h/cc             # Mix 2 audio streams
  custom_audio_source.h/cc     # Feed PCM into libwebrtc
  input_injector.h/cc          # Win32 SendInput wrapper
  msgpack_lite.h               # Minimal MessagePack decoder
```

## Files Modified

```
CLIENT/src/main.cc                    # Retry loop
CLIENT/src/peer_connection_agent.h/cc # Audio track + DataChannel + InputInjector
CLIENT/src/signaling_client.h/cc      # AddTrack for audio + reset support
CLIENT/src/websocket_transport.h/cc   # Reconnect support
CLIENT/src/config.h/cc                # Audio + control config sections
CLIENT/CMakeLists.txt                 # New source files + COM libs
WEB/viewer.html                       # Input capture + DataChannel send
```
