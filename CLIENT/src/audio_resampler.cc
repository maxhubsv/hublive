#include "audio_resampler.h"

#include <algorithm>
#include <cmath>
#include <cstdio>

bool AudioResampler::Init(int src_rate, int src_channels,
                           int dst_rate, int dst_channels) {
    if (src_rate <= 0 || dst_rate <= 0 ||
        src_channels <= 0 || dst_channels <= 0) {
        printf("[resampler] Invalid parameters: %d/%d -> %d/%d\n",
               src_rate, src_channels, dst_rate, dst_channels);
        return false;
    }

    src_rate_ = src_rate;
    src_channels_ = src_channels;
    dst_rate_ = dst_rate;
    dst_channels_ = dst_channels;
    ratio_ = static_cast<double>(dst_rate) / static_cast<double>(src_rate);
    fractional_pos_ = 0.0;
    initialized_ = true;

    printf("[resampler] %d Hz/%d ch -> %d Hz/%d ch (ratio=%.4f)\n",
           src_rate, src_channels, dst_rate, dst_channels, ratio_);
    return true;
}

int AudioResampler::MaxOutputFrames(int in_frames) const {
    if (!initialized_) return 0;
    return static_cast<int>(std::ceil(in_frames * ratio_)) + 2;
}

void AudioResampler::Process(const float* in, int in_frames,
                              float* out, int* out_frames) {
    if (!initialized_ || in_frames <= 0) {
        *out_frames = 0;
        return;
    }

    // Step 1: Down-mix to mono if src has more channels than dst wants.
    //         For simplicity, we always produce a mono intermediate buffer
    //         if dst_channels == 1 and src_channels > 1.
    const float* mono_in = in;
    int mono_channels = src_channels_;

    if (dst_channels_ == 1 && src_channels_ > 1) {
        // Down-mix to mono by averaging channels.
        mono_buf_.resize(in_frames);
        float inv = 1.0f / static_cast<float>(src_channels_);
        for (int i = 0; i < in_frames; ++i) {
            float sum = 0.0f;
            for (int ch = 0; ch < src_channels_; ++ch) {
                sum += in[i * src_channels_ + ch];
            }
            mono_buf_[i] = sum * inv;
        }
        mono_in = mono_buf_.data();
        mono_channels = 1;
    }

    // Step 2: Resample using linear interpolation.
    // For each output sample, compute the fractional position in the input
    // and interpolate between the two nearest input samples.
    if (src_rate_ == dst_rate_) {
        // No resampling needed — just copy.
        int frames = in_frames;
        for (int i = 0; i < frames; ++i) {
            for (int ch = 0; ch < dst_channels_; ++ch) {
                int src_ch = (ch < mono_channels) ? ch : 0;
                out[i * dst_channels_ + ch] = mono_in[i * mono_channels + src_ch];
            }
        }
        *out_frames = frames;
        return;
    }

    // Step size in input samples per output sample.
    double step = 1.0 / ratio_;
    double pos = fractional_pos_;
    int out_idx = 0;

    while (true) {
        int idx0 = static_cast<int>(pos);
        if (idx0 >= in_frames - 1) break;

        double frac = pos - idx0;
        int idx1 = idx0 + 1;

        for (int ch = 0; ch < dst_channels_; ++ch) {
            int src_ch = (ch < mono_channels) ? ch : 0;
            float s0 = mono_in[idx0 * mono_channels + src_ch];
            float s1 = mono_in[idx1 * mono_channels + src_ch];
            out[out_idx * dst_channels_ + ch] =
                s0 + static_cast<float>(frac) * (s1 - s0);
        }
        out_idx++;
        pos += step;
    }

    // Save fractional remainder for next call to maintain phase continuity.
    fractional_pos_ = pos - in_frames;
    if (fractional_pos_ < 0.0) fractional_pos_ = 0.0;

    *out_frames = out_idx;
}
