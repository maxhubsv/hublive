#pragma once

#include "media/base/adapted_video_track_source.h"
#include "modules/desktop_capture/desktop_capturer.h"
#include "modules/desktop_capture/desktop_capture_options.h"
#include "modules/desktop_capture/desktop_frame.h"
#include "api/scoped_refptr.h"

#include <atomic>
#include <memory>
#include <optional>
#include <thread>

class ScreenCaptureSource : public webrtc::AdaptedVideoTrackSource,
                            public webrtc::DesktopCapturer::Callback {
public:
    static webrtc::scoped_refptr<ScreenCaptureSource> Create(int monitor_index, int fps);

    ~ScreenCaptureSource() override;

    void Start();
    void Stop();

    bool is_screencast() const override { return true; }
    std::optional<bool> needs_denoising() const override { return false; }
    SourceState state() const override { return kLive; }
    bool remote() const override { return false; }

    int width() const { return width_; }
    int height() const { return height_; }

protected:
    ScreenCaptureSource(int monitor_index, int fps);

private:
    void OnCaptureResult(webrtc::DesktopCapturer::Result result,
                         std::unique_ptr<webrtc::DesktopFrame> frame) override;

    void CaptureThread();

    std::unique_ptr<webrtc::DesktopCapturer> capturer_;
    int monitor_index_;
    int fps_;
    int width_ = 0;
    int height_ = 0;
    std::atomic<bool> running_{false};
    std::unique_ptr<std::thread> capture_thread_;
};
