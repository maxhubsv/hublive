#include "audio_mixer.h"

#include <algorithm>
#include <cmath>
#include <cstring>

// ---------------------------------------------------------------------------
// RingBuffer
// ---------------------------------------------------------------------------

AudioMixer::RingBuffer::RingBuffer(size_t capacity)
    : buf_(capacity, 0.0f) {}

void AudioMixer::RingBuffer::Write(const float* data, size_t count) {
    for (size_t i = 0; i < count; ++i) {
        buf_[write_pos_] = data[i];
        write_pos_ = (write_pos_ + 1) % buf_.size();
        if (available_ < buf_.size()) {
            ++available_;
        } else {
            // Overwrite oldest sample (drop).
            read_pos_ = (read_pos_ + 1) % buf_.size();
        }
    }
}

size_t AudioMixer::RingBuffer::Read(float* data, size_t count) {
    size_t to_read = std::min(count, available_);
    for (size_t i = 0; i < to_read; ++i) {
        data[i] = buf_[read_pos_];
        read_pos_ = (read_pos_ + 1) % buf_.size();
    }
    available_ -= to_read;
    return to_read;
}

size_t AudioMixer::RingBuffer::Available() const {
    return available_;
}

// ---------------------------------------------------------------------------
// AudioMixer
// ---------------------------------------------------------------------------

AudioMixer::AudioMixer()
    : mic_buffer_(kSampleRate),      // 1 second capacity
      system_buffer_(kSampleRate) {}

void AudioMixer::SetMicGain(float gain) {
    std::lock_guard<std::mutex> lock(mutex_);
    mic_gain_ = std::clamp(gain, 0.0f, 1.0f);
}

void AudioMixer::SetSystemGain(float gain) {
    std::lock_guard<std::mutex> lock(mutex_);
    system_gain_ = std::clamp(gain, 0.0f, 1.0f);
}

void AudioMixer::PushMicData(const float* data, int frames) {
    std::lock_guard<std::mutex> lock(mutex_);
    mic_active_ = true;
    mic_buffer_.Write(data, static_cast<size_t>(frames));
}

void AudioMixer::PushSystemData(const float* data, int frames) {
    std::lock_guard<std::mutex> lock(mutex_);
    system_active_ = true;
    system_buffer_.Write(data, static_cast<size_t>(frames));
}

bool AudioMixer::GetMixedFrame(float* out, int frames) {
    std::lock_guard<std::mutex> lock(mutex_);

    // Need at least one active source with enough data.
    bool mic_ready = mic_active_ && mic_buffer_.Available() >= static_cast<size_t>(frames);
    bool sys_ready = system_active_ && system_buffer_.Available() >= static_cast<size_t>(frames);

    if (!mic_ready && !sys_ready) {
        return false;
    }

    float mic_data[kFrameSamples] = {};
    float sys_data[kFrameSamples] = {};

    size_t mic_read = 0;
    size_t sys_read = 0;

    if (mic_ready) {
        mic_read = mic_buffer_.Read(mic_data, static_cast<size_t>(frames));
    }
    if (sys_ready) {
        sys_read = system_buffer_.Read(sys_data, static_cast<size_t>(frames));
    }

    // Mix: sum with gain, clamp to [-1.0, 1.0].
    for (int i = 0; i < frames; ++i) {
        float sample = 0.0f;
        if (mic_read > 0) sample += mic_data[i] * mic_gain_;
        if (sys_read > 0) sample += sys_data[i] * system_gain_;
        out[i] = std::clamp(sample, -1.0f, 1.0f);
    }

    return true;
}
