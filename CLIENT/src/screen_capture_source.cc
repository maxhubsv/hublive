#include "screen_capture_source.h"

#include "api/make_ref_counted.h"
#include "api/video/i420_buffer.h"
#include "api/video/video_frame.h"
#include "api/video/video_rotation.h"
#include "modules/desktop_capture/desktop_and_cursor_composer.h"
#include "modules/desktop_capture/desktop_capture_types.h"
#include "rtc_base/time_utils.h"
#include "third_party/libyuv/include/libyuv/convert.h"

#include "logger.h"

#include <chrono>
#include <thread>

webrtc::scoped_refptr<ScreenCaptureSource> ScreenCaptureSource::Create(int monitor_index, int fps, bool use_directx) {
    return webrtc::make_ref_counted<ScreenCaptureSource>(monitor_index, fps, use_directx);
}

ScreenCaptureSource::ScreenCaptureSource(int monitor_index, int fps, bool use_directx)
    : monitor_index_(monitor_index), fps_(fps) {
    auto options = webrtc::DesktopCaptureOptions::CreateDefault();
    options.set_allow_directx_capturer(use_directx);

    auto screen_capturer = webrtc::DesktopCapturer::CreateScreenCapturer(options);
    if (!screen_capturer) {
        LogError("capture", "Failed to create screen capturer for monitor %d", monitor_index);
        return;
    }

    webrtc::DesktopCapturer::SourceList sources;
    if (screen_capturer->GetSourceList(&sources) && monitor_index < static_cast<int>(sources.size())) {
        screen_capturer->SelectSource(sources[monitor_index].id);
        LogInfo("capture", "Selected monitor %d: %s (directx=%s)", monitor_index,
               sources[monitor_index].title.c_str(), use_directx ? "on" : "off");
    } else if (!sources.empty()) {
        screen_capturer->SelectSource(sources[0].id);
        LogWarn("capture", "Fallback to monitor 0");
    }

    // Wrap with cursor composer to render mouse cursor on captured frames
    capturer_ = std::make_unique<webrtc::DesktopAndCursorComposer>(
        std::move(screen_capturer), options);
    LogInfo("capture", "Mouse cursor rendering enabled");
}

ScreenCaptureSource::~ScreenCaptureSource() {
    Stop();
}

void ScreenCaptureSource::Start() {
    if (!capturer_ || running_) return;

    capturer_->Start(this);
    running_ = true;
    capture_thread_ = std::make_unique<std::thread>(&ScreenCaptureSource::CaptureThread, this);
    LogInfo("capture", "Started @ %d fps", fps_);
}

void ScreenCaptureSource::Stop() {
    running_ = false;
    if (capture_thread_ && capture_thread_->joinable()) {
        capture_thread_->join();
    }
    capture_thread_.reset();
    LogInfo("capture", "Stopped");
}

void ScreenCaptureSource::CaptureThread() {
    auto interval = std::chrono::milliseconds(1000 / fps_);

    while (running_) {
        auto start = std::chrono::steady_clock::now();
        capturer_->CaptureFrame();
        auto elapsed = std::chrono::steady_clock::now() - start;
        auto sleep_time = interval - elapsed;
        if (sleep_time > std::chrono::milliseconds(0)) {
            std::this_thread::sleep_for(sleep_time);
        }
    }
}

void ScreenCaptureSource::OnCaptureResult(
    webrtc::DesktopCapturer::Result result,
    std::unique_ptr<webrtc::DesktopFrame> frame) {

    if (result != webrtc::DesktopCapturer::Result::SUCCESS || !frame) {
        return;
    }

    int width = frame->size().width();
    int height = frame->size().height();
    width_ = width;
    height_ = height;

    webrtc::scoped_refptr<webrtc::I420Buffer> i420_buffer =
        webrtc::I420Buffer::Create(width, height);

    // DesktopFrame is BGRA on Windows. libyuv::ARGBToI420 expects ARGB byte
    // order. On little-endian (x86), BGRA in memory == ARGB when read as
    // uint32, so ARGBToI420 is the correct conversion function.
    libyuv::ARGBToI420(
        frame->data(), frame->stride(),
        i420_buffer->MutableDataY(), i420_buffer->StrideY(),
        i420_buffer->MutableDataU(), i420_buffer->StrideU(),
        i420_buffer->MutableDataV(), i420_buffer->StrideV(),
        width, height);

    webrtc::VideoFrame video_frame =
        webrtc::VideoFrame::Builder()
            .set_video_frame_buffer(i420_buffer)
            .set_timestamp_us(webrtc::TimeMicros())
            .set_rotation(webrtc::kVideoRotation_0)
            .build();

    OnFrame(video_frame);
}
