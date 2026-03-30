#pragma once

// Simple linear-interpolation audio resampler.
//
// Converts between sample rates and optionally down-mixes to mono.
// No external dependencies (no speexdsp).

#include <vector>

class AudioResampler {
public:
    AudioResampler() = default;

    // Initialize the resampler.
    //   src_rate/src_channels: input format
    //   dst_rate/dst_channels: output format
    bool Init(int src_rate, int src_channels, int dst_rate, int dst_channels);

    // Resample a block of float32 interleaved samples.
    //   in:         input buffer (src_channels interleaved)
    //   in_frames:  number of frames in input
    //   out:        output buffer (must be large enough: see MaxOutputFrames)
    //   out_frames: [out] actual frames written
    void Process(const float* in, int in_frames, float* out, int* out_frames);

    // Upper bound on output frames for a given input frame count.
    int MaxOutputFrames(int in_frames) const;

    bool initialized() const { return initialized_; }

private:
    bool initialized_ = false;
    int src_rate_ = 0;
    int src_channels_ = 0;
    int dst_rate_ = 0;
    int dst_channels_ = 0;
    double ratio_ = 1.0;  // dst_rate / src_rate

    // Fractional sample position carried across calls for continuity.
    double fractional_pos_ = 0.0;

    // Intermediate mono buffer (used when down-mixing before resampling).
    std::vector<float> mono_buf_;
};
