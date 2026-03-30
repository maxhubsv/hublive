#pragma once

// AudioMixer — mixes two mono audio streams (mic + system) into one.
//
// Each source pushes resampled 48 kHz mono float32 frames.
// GetMixedFrame() produces 10ms frames (480 samples at 48 kHz).

#include <algorithm>
#include <cstring>
#include <mutex>
#include <vector>

class AudioMixer {
public:
    static constexpr int kSampleRate = 48000;
    static constexpr int kChannels = 1;
    static constexpr int kFrameDurationMs = 10;
    static constexpr int kFrameSamples = kSampleRate * kFrameDurationMs / 1000;  // 480

    AudioMixer();

    void SetMicGain(float gain);
    void SetSystemGain(float gain);

    // Push resampled mono float32 data from mic or system capture.
    void PushMicData(const float* data, int frames);
    void PushSystemData(const float* data, int frames);

    // Get a mixed 10ms frame (480 mono samples). Returns false if insufficient data.
    bool GetMixedFrame(float* out, int frames);

private:
    // Simple ring buffer for a single audio stream.
    class RingBuffer {
    public:
        explicit RingBuffer(size_t capacity = 48000);  // 1 second default

        void Write(const float* data, size_t count);
        size_t Read(float* data, size_t count);
        size_t Available() const;

    private:
        std::vector<float> buf_;
        size_t read_pos_ = 0;
        size_t write_pos_ = 0;
        size_t available_ = 0;
    };

    std::mutex mutex_;
    RingBuffer mic_buffer_;
    RingBuffer system_buffer_;
    float mic_gain_ = 1.0f;
    float system_gain_ = 0.8f;

    // Whether each source has ever received data (for single-source mixing).
    bool mic_active_ = false;
    bool system_active_ = false;
};
