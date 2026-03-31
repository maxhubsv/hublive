#include "audio_capture.h"
#include "logger.h"

#include <cstring>
#include <vector>

// Link COM libraries.
#pragma comment(lib, "ole32.lib")
#pragma comment(lib, "uuid.lib")

// WASAPI buffer duration: 20ms (in 100-nanosecond units).
static const REFERENCE_TIME kBufferDuration = 200000;

// Polling interval for capture thread.
static const int kPollIntervalMs = 10;

WasapiCapture::WasapiCapture() = default;

WasapiCapture::~WasapiCapture() {
    Stop();

    if (capture_client_) { capture_client_->Release(); capture_client_ = nullptr; }
    if (audio_client_) { audio_client_->Release(); audio_client_ = nullptr; }
    if (device_) { device_->Release(); device_ = nullptr; }
}

bool WasapiCapture::Init(bool loopback) {
    loopback_ = loopback;

    // COM must be initialized on this thread.
    HRESULT hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
    if (FAILED(hr) && hr != RPC_E_CHANGED_MODE && hr != S_FALSE) {
        LogError("audio", "CoInitializeEx failed: 0x%08lx", hr);
        return false;
    }

    // Get default audio endpoint.
    IMMDeviceEnumerator* enumerator = nullptr;
    hr = CoCreateInstance(__uuidof(MMDeviceEnumerator), nullptr,
                          CLSCTX_ALL, __uuidof(IMMDeviceEnumerator),
                          reinterpret_cast<void**>(&enumerator));
    if (FAILED(hr) || !enumerator) {
        LogError("audio", "Failed to create device enumerator: 0x%08lx", hr);
        return false;
    }

    // Loopback captures from a render endpoint; mic from a capture endpoint.
    EDataFlow data_flow = loopback ? eRender : eCapture;
    hr = enumerator->GetDefaultAudioEndpoint(data_flow, eConsole, &device_);
    enumerator->Release();

    if (FAILED(hr) || !device_) {
        LogError("audio", "No default %s device found: 0x%08lx",
               loopback ? "render" : "capture", hr);
        return false;
    }

    // Print device name for diagnostics.
    IPropertyStore* props = nullptr;
    if (SUCCEEDED(device_->OpenPropertyStore(STGM_READ, &props)) && props) {
        PROPVARIANT var;
        PropVariantInit(&var);
        if (SUCCEEDED(props->GetValue(PKEY_Device_FriendlyName, &var))) {
            if (var.vt == VT_LPWSTR && var.pwszVal) {
                LogInfo("audio", "%s device: %ls",
                       loopback ? "Loopback" : "Mic", var.pwszVal);
            }
            PropVariantClear(&var);
        }
        props->Release();
    }

    // Activate audio client.
    hr = device_->Activate(__uuidof(IAudioClient), CLSCTX_ALL,
                           nullptr, reinterpret_cast<void**>(&audio_client_));
    if (FAILED(hr) || !audio_client_) {
        LogError("audio", "Failed to activate audio client: 0x%08lx", hr);
        return false;
    }

    // Get the mix format (native format of the endpoint).
    WAVEFORMATEX* mix_format = nullptr;
    hr = audio_client_->GetMixFormat(&mix_format);
    if (FAILED(hr) || !mix_format) {
        LogError("audio", "GetMixFormat failed: 0x%08lx", hr);
        return false;
    }

    sample_rate_ = mix_format->nSamplesPerSec;
    channels_ = mix_format->nChannels;
    bits_per_sample_ = mix_format->wBitsPerSample;

    LogInfo("audio", "%s format: %d Hz, %d ch, %d bits",
           loopback ? "Loopback" : "Mic",
           sample_rate_, channels_, bits_per_sample_);

    // Ensure float32 format. WASAPI shared mode typically provides
    // WAVE_FORMAT_EXTENSIBLE with IEEE_FLOAT subformat.
    // If it's PCM16, we'll need to convert (handled in capture thread).

    // Loopback: use AUDCLNT_STREAMFLAGS_LOOPBACK to capture system audio.
    // Mic: no special flags needed (we use a polling model, not event-driven,
    // so AUDCLNT_STREAMFLAGS_EVENTCALLBACK must NOT be set without a
    // corresponding SetEventHandle() call).
    DWORD stream_flags = 0;
    if (loopback) {
        stream_flags = AUDCLNT_STREAMFLAGS_LOOPBACK;
    }

    // Initialize the audio client in shared mode.
    hr = audio_client_->Initialize(
        AUDCLNT_SHAREMODE_SHARED,
        stream_flags,
        kBufferDuration,
        0,  // periodicity (must be 0 for shared mode)
        mix_format,
        nullptr);  // session GUID

    CoTaskMemFree(mix_format);

    if (FAILED(hr)) {
        LogError("audio", "AudioClient Initialize failed: 0x%08lx", hr);
        return false;
    }

    // Get the capture client interface.
    hr = audio_client_->GetService(__uuidof(IAudioCaptureClient),
                                   reinterpret_cast<void**>(&capture_client_));
    if (FAILED(hr) || !capture_client_) {
        LogError("audio", "Failed to get capture client: 0x%08lx", hr);
        return false;
    }

    LogInfo("audio", "%s capture initialized", loopback ? "Loopback" : "Mic");
    return true;
}

bool WasapiCapture::Start() {
    if (!audio_client_ || !capture_client_) return false;
    if (running_) return true;

    HRESULT hr = audio_client_->Start();
    if (FAILED(hr)) {
        LogError("audio", "AudioClient Start failed: 0x%08lx", hr);
        return false;
    }

    running_ = true;
    capture_thread_ = std::thread(&WasapiCapture::CaptureThread, this);
    LogInfo("audio", "%s capture started", loopback_ ? "Loopback" : "Mic");
    return true;
}

void WasapiCapture::Stop() {
    running_ = false;
    if (capture_thread_.joinable()) {
        capture_thread_.join();
    }
    if (audio_client_) {
        audio_client_->Stop();
    }
}

void WasapiCapture::CaptureThread() {
    // Each capture thread needs its own COM initialization.
    CoInitializeEx(nullptr, COINIT_MULTITHREADED);

    // Temporary buffer for int16->float conversion.
    std::vector<float> float_buf;

    while (running_) {
        UINT32 packet_length = 0;
        HRESULT hr = capture_client_->GetNextPacketSize(&packet_length);
        if (FAILED(hr)) {
            LogError("audio", "GetNextPacketSize failed: 0x%08lx", hr);
            break;
        }

        while (packet_length > 0) {
            BYTE* data = nullptr;
            UINT32 frames_available = 0;
            DWORD flags = 0;

            hr = capture_client_->GetBuffer(&data, &frames_available, &flags,
                                            nullptr, nullptr);
            if (FAILED(hr)) {
                LogError("audio", "GetBuffer failed: 0x%08lx", hr);
                break;
            }

            if (on_data_ && frames_available > 0) {
                if (flags & AUDCLNT_BUFFERFLAGS_SILENT) {
                    // Deliver silence.
                    float_buf.assign(static_cast<size_t>(frames_available) * channels_, 0.0f);
                    on_data_(float_buf.data(), frames_available, channels_, sample_rate_);
                } else if (bits_per_sample_ == 32) {
                    // Assume float32 (WASAPI shared mode default).
                    on_data_(reinterpret_cast<const float*>(data),
                             frames_available, channels_, sample_rate_);
                } else if (bits_per_sample_ == 16) {
                    // Convert int16 to float32.
                    size_t total_samples = static_cast<size_t>(frames_available) * channels_;
                    float_buf.resize(total_samples);
                    const int16_t* src = reinterpret_cast<const int16_t*>(data);
                    for (size_t i = 0; i < total_samples; ++i) {
                        float_buf[i] = static_cast<float>(src[i]) / 32768.0f;
                    }
                    on_data_(float_buf.data(), frames_available, channels_, sample_rate_);
                }
                // Other bit depths are rare in shared mode; silently skip.
            }

            capture_client_->ReleaseBuffer(frames_available);

            hr = capture_client_->GetNextPacketSize(&packet_length);
            if (FAILED(hr)) break;
        }

        // Sleep briefly before polling again.
        std::this_thread::sleep_for(std::chrono::milliseconds(kPollIntervalMs));
    }

    CoUninitialize();
}
