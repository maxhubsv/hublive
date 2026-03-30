#include "custom_audio_source.h"

#include <algorithm>
#include <cstdio>

CustomAudioSource::CustomAudioSource() {
    // Disable all audio processing since we provide raw mixed audio.
    options_.echo_cancellation = false;
    options_.auto_gain_control = false;
    options_.noise_suppression = false;
    options_.highpass_filter = false;
    options_.stereo_swapping = false;
    options_.typing_detection = false;
}

CustomAudioSource::~CustomAudioSource() = default;

void CustomAudioSource::AddSink(webrtc::AudioTrackSinkInterface* sink) {
    if (!sink) return;
    std::lock_guard<std::mutex> lock(sinks_mutex_);
    sinks_.push_back(sink);
}

void CustomAudioSource::RemoveSink(webrtc::AudioTrackSinkInterface* sink) {
    if (!sink) return;
    std::lock_guard<std::mutex> lock(sinks_mutex_);
    sinks_.erase(
        std::remove(sinks_.begin(), sinks_.end(), sink),
        sinks_.end());
}

void CustomAudioSource::DeliverData(const int16_t* data, int frames,
                                     int sample_rate, int channels) {
    std::lock_guard<std::mutex> lock(sinks_mutex_);
    for (auto* sink : sinks_) {
        // AudioTrackSinkInterface::OnData expects:
        //   audio_data:         pointer to interleaved PCM
        //   bits_per_sample:    16 for int16
        //   sample_rate:        e.g. 48000
        //   number_of_channels: e.g. 1
        //   number_of_frames:   e.g. 480
        sink->OnData(data, 16, sample_rate,
                     static_cast<size_t>(channels),
                     static_cast<size_t>(frames));
    }
}
