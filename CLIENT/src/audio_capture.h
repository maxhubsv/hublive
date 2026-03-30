#pragma once

// WASAPI audio capture — loopback (system audio) or microphone.
//
// Usage:
//   WasapiCapture cap;
//   cap.SetOnData([](const float* data, int frames, int channels, int sample_rate) { ... });
//   if (cap.Init(true)) {  // true = loopback
//       cap.Start();
//       // ...
//       cap.Stop();
//   }

#include <windows.h>
#include <mmdeviceapi.h>
#include <audioclient.h>
#include <functiondiscoverykeys_devpkey.h>

#include <atomic>
#include <functional>
#include <string>
#include <thread>

class WasapiCapture {
public:
    using OnDataCallback = std::function<void(const float* data, int frames,
                                              int channels, int sample_rate)>;

    WasapiCapture();
    ~WasapiCapture();

    // Initialize WASAPI endpoint.
    //   loopback = true  -> system audio (eRender + AUDCLNT_STREAMFLAGS_LOOPBACK)
    //   loopback = false -> default microphone (eCapture)
    bool Init(bool loopback);

    bool Start();
    void Stop();

    void SetOnData(OnDataCallback cb) { on_data_ = std::move(cb); }

    int sample_rate() const { return sample_rate_; }
    int channels() const { return channels_; }
    bool is_loopback() const { return loopback_; }

private:
    void CaptureThread();

    bool loopback_ = false;
    int sample_rate_ = 0;
    int channels_ = 0;
    int bits_per_sample_ = 0;

    IMMDevice* device_ = nullptr;
    IAudioClient* audio_client_ = nullptr;
    IAudioCaptureClient* capture_client_ = nullptr;

    std::atomic<bool> running_{false};
    std::thread capture_thread_;

    OnDataCallback on_data_;
};
