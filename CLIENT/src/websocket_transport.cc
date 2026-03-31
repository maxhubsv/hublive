#include "websocket_transport.h"
#include "logger.h"
#include <sstream>

#pragma comment(lib, "winhttp.lib")

static bool ParseWsUrl(const std::string& url,
                       std::wstring& host, INTERNET_PORT& port, std::wstring& path) {
    std::string work = url;

    if (work.find("wss://") == 0) {
        work = work.substr(6);
        port = 443;
    } else if (work.find("ws://") == 0) {
        work = work.substr(5);
        port = 80;
    } else {
        return false;
    }

    auto slash_pos = work.find('/');
    std::string host_port = (slash_pos != std::string::npos) ? work.substr(0, slash_pos) : work;
    std::string path_str = (slash_pos != std::string::npos) ? work.substr(slash_pos) : "/rtc";

    auto colon_pos = host_port.find(':');
    std::string host_str;
    if (colon_pos != std::string::npos) {
        host_str = host_port.substr(0, colon_pos);
        port = static_cast<INTERNET_PORT>(std::stoi(host_port.substr(colon_pos + 1)));
    } else {
        host_str = host_port;
    }

    host.assign(host_str.begin(), host_str.end());
    path.assign(path_str.begin(), path_str.end());
    return true;
}

static std::wstring Utf8ToWide(const std::string& s) {
    if (s.empty()) return L"";
    int len = MultiByteToWideChar(CP_UTF8, 0, s.c_str(), -1, nullptr, 0);
    std::wstring ws(len - 1, 0);
    MultiByteToWideChar(CP_UTF8, 0, s.c_str(), -1, &ws[0], len);
    return ws;
}

WebSocketTransport::WebSocketTransport() {}

WebSocketTransport::~WebSocketTransport() {
    Close();
}

bool WebSocketTransport::Connect(const std::string& url, const std::string& auth_token) {
    std::wstring host;
    INTERNET_PORT port;
    std::wstring path;

    if (!ParseWsUrl(url, host, port, path)) {
        if (on_error_) on_error_("Invalid WebSocket URL: " + url);
        return false;
    }

    if (path == L"/") path = L"/rtc";
    path += L"?protocol=16&auto_subscribe=false&sdk=cpp";

    session_ = WinHttpOpen(L"HubLiveAgent/1.0",
                           WINHTTP_ACCESS_TYPE_NO_PROXY,
                           WINHTTP_NO_PROXY_NAME,
                           WINHTTP_NO_PROXY_BYPASS, 0);
    if (!session_) {
        if (on_error_) on_error_("WinHttpOpen failed: " + std::to_string(GetLastError()));
        return false;
    }

    connection_ = WinHttpConnect(session_, host.c_str(), port, 0);
    if (!connection_) {
        if (on_error_) on_error_("WinHttpConnect failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    request_ = WinHttpOpenRequest(connection_, L"GET", path.c_str(),
                                   nullptr, WINHTTP_NO_REFERER,
                                   WINHTTP_DEFAULT_ACCEPT_TYPES, 0);
    if (!request_) {
        if (on_error_) on_error_("WinHttpOpenRequest failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    if (!WinHttpSetOption(request_, WINHTTP_OPTION_UPGRADE_TO_WEB_SOCKET, nullptr, 0)) {
        if (on_error_) on_error_("Failed to set WebSocket upgrade option");
        Cleanup();
        return false;
    }

    std::wstring auth_header = L"Authorization: Bearer " + Utf8ToWide(auth_token);
    WinHttpAddRequestHeaders(request_, auth_header.c_str(), -1L, WINHTTP_ADDREQ_FLAG_ADD);

    if (!WinHttpSendRequest(request_, WINHTTP_NO_ADDITIONAL_HEADERS, 0,
                            WINHTTP_NO_REQUEST_DATA, 0, 0, 0)) {
        if (on_error_) on_error_("WinHttpSendRequest failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    if (!WinHttpReceiveResponse(request_, nullptr)) {
        if (on_error_) on_error_("WinHttpReceiveResponse failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    // Check HTTP status code
    DWORD status_code = 0;
    DWORD status_size = sizeof(status_code);
    WinHttpQueryHeaders(request_,
                        WINHTTP_QUERY_STATUS_CODE | WINHTTP_QUERY_FLAG_NUMBER,
                        WINHTTP_HEADER_NAME_BY_INDEX,
                        &status_code, &status_size, WINHTTP_NO_HEADER_INDEX);
    if (status_code != 101) {
        // Read response body for error details
        DWORD bytes_available = 0;
        std::string body;
        WinHttpQueryDataAvailable(request_, &bytes_available);
        if (bytes_available > 0 && bytes_available < 4096) {
            std::vector<char> buf(bytes_available + 1, 0);
            DWORD bytes_read = 0;
            WinHttpReadData(request_, buf.data(), bytes_available, &bytes_read);
            body.assign(buf.data(), bytes_read);
        }
        if (on_error_) on_error_("HTTP " + std::to_string(status_code) + ": " + body);
        Cleanup();
        return false;
    }

    websocket_ = WinHttpWebSocketCompleteUpgrade(request_, 0);
    if (!websocket_) {
        if (on_error_) on_error_("WebSocket upgrade failed: " + std::to_string(GetLastError()));
        Cleanup();
        return false;
    }

    WinHttpCloseHandle(request_);
    request_ = nullptr;

    connected_ = true;
    recv_thread_ = std::thread(&WebSocketTransport::RecvLoop, this);

    return true;
}

bool WebSocketTransport::Send(const std::vector<uint8_t>& data) {
    return Send(data.data(), data.size());
}

bool WebSocketTransport::Send(const uint8_t* data, size_t len) {
    if (!connected_ || !websocket_) return false;

    std::lock_guard<std::mutex> lock(send_mutex_);
    DWORD err = WinHttpWebSocketSend(websocket_,
                                      WINHTTP_WEB_SOCKET_BINARY_MESSAGE_BUFFER_TYPE,
                                      (PVOID)data, static_cast<DWORD>(len));
    if (err != ERROR_SUCCESS) {
        LogError("ws", "WebSocket send error: %lu", err);
        return false;
    }
    return true;
}

void WebSocketTransport::RecvLoop() {
    std::vector<uint8_t> buffer(64 * 1024);
    std::vector<uint8_t> message;

    while (connected_ && websocket_) {
        DWORD bytes_read = 0;
        WINHTTP_WEB_SOCKET_BUFFER_TYPE buffer_type;

        DWORD err = WinHttpWebSocketReceive(websocket_,
                                             buffer.data(),
                                             static_cast<DWORD>(buffer.size()),
                                             &bytes_read, &buffer_type);
        if (err != ERROR_SUCCESS) {
            if (connected_) {
                connected_ = false;
                if (on_error_) on_error_("WebSocket recv error: " + std::to_string(err));
            }
            break;
        }

        if (buffer_type == WINHTTP_WEB_SOCKET_CLOSE_BUFFER_TYPE) {
            connected_ = false;
            if (on_close_) on_close_(0, "Server closed connection");
            break;
        }

        if (buffer_type == WINHTTP_WEB_SOCKET_BINARY_MESSAGE_BUFFER_TYPE ||
            buffer_type == WINHTTP_WEB_SOCKET_BINARY_FRAGMENT_BUFFER_TYPE) {
            message.insert(message.end(), buffer.begin(), buffer.begin() + bytes_read);

            if (buffer_type == WINHTTP_WEB_SOCKET_BINARY_MESSAGE_BUFFER_TYPE) {
                if (on_message_) on_message_(message);
                message.clear();
            }
        }
    }
}

void WebSocketTransport::Close() {
    connected_ = false;

    if (websocket_) {
        WinHttpWebSocketClose(websocket_, WINHTTP_WEB_SOCKET_SUCCESS_CLOSE_STATUS, nullptr, 0);
    }

    if (recv_thread_.joinable()) {
        recv_thread_.join();
    }

    Cleanup();
}

void WebSocketTransport::Cleanup() {
    if (websocket_) { WinHttpCloseHandle(websocket_); websocket_ = nullptr; }
    if (request_) { WinHttpCloseHandle(request_); request_ = nullptr; }
    if (connection_) { WinHttpCloseHandle(connection_); connection_ = nullptr; }
    if (session_) { WinHttpCloseHandle(session_); session_ = nullptr; }
}

void WebSocketTransport::Reset() {
    // Ensure everything is closed first.
    Close();

    // All handles are already nullptr after Close()->Cleanup().
    // Reset connected flag (should already be false after Close()).
    connected_ = false;

    // Note: callbacks (on_message_, on_close_, on_error_) are preserved
    // intentionally so the caller does not need to re-register them.
}
