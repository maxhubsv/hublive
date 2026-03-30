#pragma once

// CustomAudioSource — feeds raw PCM audio into a libwebrtc audio track.
//
// Implements webrtc::AudioSourceInterface so that it can be used with
// PeerConnectionFactory::CreateAudioTrack(). Audio data pushed via
// OnData() is forwarded to all registered AudioTrackSinkInterface sinks
// (which is how libwebrtc routes audio to the encoder).

#include "api/media_stream_interface.h"
#include "api/notifier.h"
#include "api/audio_options.h"

#include <mutex>
#include <vector>

class CustomAudioSource : public webrtc::Notifier<webrtc::AudioSourceInterface> {
public:
    CustomAudioSource();
    ~CustomAudioSource() override;

    // Push audio data to all registered sinks.
    //   data:        interleaved PCM samples (int16 format)
    //   frames:      number of frames
    //   sample_rate: e.g. 48000
    //   channels:    e.g. 1 (mono)
    void DeliverData(const int16_t* data, int frames,
                     int sample_rate, int channels);

    // AudioSourceInterface overrides.
    SourceState state() const override { return kLive; }
    bool remote() const override { return false; }
    const webrtc::AudioOptions options() const override { return options_; }

    void AddSink(webrtc::AudioTrackSinkInterface* sink) override;
    void RemoveSink(webrtc::AudioTrackSinkInterface* sink) override;

private:
    webrtc::AudioOptions options_;
    std::mutex sinks_mutex_;
    std::vector<webrtc::AudioTrackSinkInterface*> sinks_;
};
