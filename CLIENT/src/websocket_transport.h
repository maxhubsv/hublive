#pragma once

#include <windows.h>
#include <winhttp.h>
#include <string>
#include <vector>
#include <functional>
#include <thread>
#include <atomic>
#include <mutex>

class WebSocketTransport {
public:
    using OnMessageCallback = std::function<void(const std::vector<uint8_t>& data)>;
    using OnCloseCallback = std::function<void(int code, const std::string& reason)>;
    using OnErrorCallback = std::function<void(const std::string& error)>;

    WebSocketTransport();
    ~WebSocketTransport();

    bool Connect(const std::string& url, const std::string& auth_token);
    bool Send(const std::vector<uint8_t>& data);
    bool Send(const uint8_t* data, size_t len);
    void Close();
    bool IsConnected() const { return connected_.load(); }

    void SetOnMessage(OnMessageCallback cb) { on_message_ = std::move(cb); }
    void SetOnClose(OnCloseCallback cb) { on_close_ = std::move(cb); }
    void SetOnError(OnErrorCallback cb) { on_error_ = std::move(cb); }

private:
    void RecvLoop();
    void Cleanup();

    HINTERNET session_ = nullptr;
    HINTERNET connection_ = nullptr;
    HINTERNET request_ = nullptr;
    HINTERNET websocket_ = nullptr;

    std::thread recv_thread_;
    std::atomic<bool> connected_{false};
    std::mutex send_mutex_;

    OnMessageCallback on_message_;
    OnCloseCallback on_close_;
    OnErrorCallback on_error_;
};
