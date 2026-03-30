/*
 * HubLive Screen Streaming Agent
 *
 * whip_client.cc — WHIP client using WinHTTP for HTTP requests on Windows.
 *
 * WHIP protocol (RFC 9725):
 *   POST   /whip/v1              → SDP offer  → SDP answer (201 Created)
 *   PATCH  /whip/v1/{id}         → ICE candidates (trickle)
 *   DELETE /whip/v1/{id}         → Close session
 */

#include "whip_client.h"

#include <cstring>
#include <sstream>

#include "rtc_base/logging.h"

#if defined(WEBRTC_WIN)
#include <windows.h>
#include <winhttp.h>
#pragma comment(lib, "winhttp.lib")
#endif

namespace hublive {

namespace {

#if defined(WEBRTC_WIN)

struct UrlParts {
  bool is_https;
  std::wstring host;
  int port;
  std::wstring path;
};

std::wstring Utf8ToWide(const std::string& utf8) {
  if (utf8.empty()) return {};
  int size = MultiByteToWideChar(CP_UTF8, 0, utf8.data(),
                                 static_cast<int>(utf8.size()), nullptr, 0);
  std::wstring wide(size, 0);
  MultiByteToWideChar(CP_UTF8, 0, utf8.data(),
                      static_cast<int>(utf8.size()), &wide[0], size);
  return wide;
}

std::string WideToUtf8(const std::wstring& wide) {
  if (wide.empty()) return {};
  int size = WideCharToMultiByte(CP_UTF8, 0, wide.data(),
                                 static_cast<int>(wide.size()), nullptr, 0,
                                 nullptr, nullptr);
  std::string utf8(size, 0);
  WideCharToMultiByte(CP_UTF8, 0, wide.data(),
                      static_cast<int>(wide.size()), &utf8[0], size,
                      nullptr, nullptr);
  return utf8;
}

bool ParseUrl(const std::string& url, UrlParts* parts) {
  std::wstring wurl = Utf8ToWide(url);
  URL_COMPONENTS uc = {};
  uc.dwStructSize = sizeof(uc);
  wchar_t host[256] = {};
  wchar_t path[1024] = {};
  uc.lpszHostName = host;
  uc.dwHostNameLength = 256;
  uc.lpszUrlPath = path;
  uc.dwUrlPathLength = 1024;

  if (!WinHttpCrackUrl(wurl.c_str(), 0, 0, &uc)) return false;

  parts->is_https = (uc.nScheme == INTERNET_SCHEME_HTTPS);
  parts->host = host;
  parts->port = uc.nPort;
  parts->path = path;
  return true;
}

// Simple WinHTTP request helper. Returns HTTP status code (0 on failure).
int HttpRequest(const std::string& method,
                const std::string& url,
                const std::string& content_type,
                const std::string& body,
                const std::string& auth_header,
                const std::string& extra_headers,
                std::string* response_body,
                std::string* response_headers_out) {
  UrlParts parts;
  if (!ParseUrl(url, &parts)) {
    RTC_LOG(LS_ERROR) << "WHIP: Failed to parse URL: " << url;
    return 0;
  }

  HINTERNET session = WinHttpOpen(L"HubLive-Agent/1.0",
                                  WINHTTP_ACCESS_TYPE_DEFAULT_PROXY,
                                  WINHTTP_NO_PROXY_NAME,
                                  WINHTTP_NO_PROXY_BYPASS, 0);
  if (!session) return 0;

  HINTERNET connect = WinHttpConnect(session, parts.host.c_str(),
                                     parts.port, 0);
  if (!connect) { WinHttpCloseHandle(session); return 0; }

  std::wstring wmethod = Utf8ToWide(method);
  DWORD flags = parts.is_https ? WINHTTP_FLAG_SECURE : 0;
  HINTERNET request = WinHttpOpenRequest(connect, wmethod.c_str(),
                                         parts.path.c_str(), nullptr,
                                         WINHTTP_NO_REFERER,
                                         WINHTTP_DEFAULT_ACCEPT_TYPES,
                                         flags);
  if (!request) {
    WinHttpCloseHandle(connect);
    WinHttpCloseHandle(session);
    return 0;
  }

  // Add headers
  std::wstring headers;
  if (!content_type.empty()) {
    headers += L"Content-Type: " + Utf8ToWide(content_type) + L"\r\n";
  }
  if (!auth_header.empty()) {
    headers += L"Authorization: Bearer " + Utf8ToWide(auth_header) + L"\r\n";
  }
  if (!extra_headers.empty()) {
    headers += Utf8ToWide(extra_headers);
  }

  if (!headers.empty()) {
    WinHttpAddRequestHeaders(request, headers.c_str(),
                             static_cast<DWORD>(headers.size()),
                             WINHTTP_ADDREQ_FLAG_ADD);
  }

  BOOL sent = WinHttpSendRequest(
      request, WINHTTP_NO_ADDITIONAL_HEADERS, 0,
      body.empty() ? WINHTTP_NO_REQUEST_DATA : const_cast<char*>(body.data()),
      static_cast<DWORD>(body.size()),
      static_cast<DWORD>(body.size()), 0);

  if (!sent || !WinHttpReceiveResponse(request, nullptr)) {
    WinHttpCloseHandle(request);
    WinHttpCloseHandle(connect);
    WinHttpCloseHandle(session);
    return 0;
  }

  // Get status code
  DWORD status_code = 0;
  DWORD status_size = sizeof(status_code);
  WinHttpQueryHeaders(request, WINHTTP_QUERY_STATUS_CODE |
                      WINHTTP_QUERY_FLAG_NUMBER, WINHTTP_HEADER_NAME_BY_INDEX,
                      &status_code, &status_size, WINHTTP_NO_HEADER_INDEX);

  // Read all response headers
  if (response_headers_out) {
    DWORD hdr_size = 0;
    WinHttpQueryHeaders(request, WINHTTP_QUERY_RAW_HEADERS_CRLF,
                        WINHTTP_HEADER_NAME_BY_INDEX, nullptr,
                        &hdr_size, WINHTTP_NO_HEADER_INDEX);
    if (hdr_size > 0) {
      std::vector<wchar_t> hdr_buf(hdr_size / sizeof(wchar_t));
      WinHttpQueryHeaders(request, WINHTTP_QUERY_RAW_HEADERS_CRLF,
                          WINHTTP_HEADER_NAME_BY_INDEX, hdr_buf.data(),
                          &hdr_size, WINHTTP_NO_HEADER_INDEX);
      *response_headers_out = WideToUtf8(std::wstring(hdr_buf.data()));
    }
  }

  // Read response body
  if (response_body) {
    DWORD bytes_available = 0;
    do {
      WinHttpQueryDataAvailable(request, &bytes_available);
      if (bytes_available == 0) break;
      std::vector<char> buf(bytes_available);
      DWORD bytes_read = 0;
      WinHttpReadData(request, buf.data(), bytes_available, &bytes_read);
      response_body->append(buf.data(), bytes_read);
    } while (bytes_available > 0);
  }

  WinHttpCloseHandle(request);
  WinHttpCloseHandle(connect);
  WinHttpCloseHandle(session);
  return static_cast<int>(status_code);
}

// Extract a header value from raw headers string.
std::string ExtractHeader(const std::string& headers,
                          const std::string& name) {
  // Case-insensitive search.
  std::string search = "\r\n" + name + ": ";
  std::string lower_headers = headers;
  std::string lower_search = search;
  for (auto& c : lower_headers) c = static_cast<char>(tolower(c));
  for (auto& c : lower_search) c = static_cast<char>(tolower(c));

  size_t pos = lower_headers.find(lower_search);
  if (pos == std::string::npos) return {};

  size_t start = pos + search.size();
  size_t end = headers.find("\r\n", start);
  if (end == std::string::npos) end = headers.size();
  return headers.substr(start, end - start);
}

#endif  // WEBRTC_WIN

}  // namespace

WhipClient::WhipClient(const WhipConfig& config) : config_(config) {}

WhipClient::~WhipClient() = default;

bool WhipClient::Publish(const std::string& offer_sdp, WhipSession* session) {
#if defined(WEBRTC_WIN)
  std::string url = config_.server_url + "/whip/v1";
  std::string response_body;
  std::string response_headers;

  RTC_LOG(LS_INFO) << "WHIP: POST " << url;

  int status = HttpRequest("POST", url, "application/sdp", offer_sdp,
                           config_.bearer_token, "",
                           &response_body, &response_headers);

  if (status != 201) {
    RTC_LOG(LS_ERROR) << "WHIP: Publish failed, HTTP " << status;
    if (!response_body.empty()) {
      RTC_LOG(LS_ERROR) << "WHIP: Response: " << response_body;
    }
    return false;
  }

  session->answer_sdp = response_body;
  session->participant_id = ExtractHeader(response_headers, "Location");
  session->etag = ExtractHeader(response_headers, "ETag");

  // Strip quotes from ETag if present.
  if (session->etag.size() >= 2 && session->etag.front() == '"') {
    session->etag = session->etag.substr(1, session->etag.size() - 2);
  }

  RTC_LOG(LS_INFO) << "WHIP: Published, participant=" << session->participant_id
                    << " etag=" << session->etag;
  return true;
#else
  RTC_LOG(LS_ERROR) << "WHIP: Only Windows is supported";
  return false;
#endif
}

bool WhipClient::TrickleIce(const std::string& participant_id,
                             const std::string& etag,
                             const std::string& ice_fragment) {
#if defined(WEBRTC_WIN)
  std::string url = config_.server_url + participant_id;
  std::string extra_headers = "If-Match: \"" + etag + "\"\r\n";
  std::string response_body;

  int status = HttpRequest("PATCH", url, "application/trickle-ice-sdpfrag",
                           ice_fragment, config_.bearer_token, extra_headers,
                           &response_body, nullptr);
  return status == 204 || status == 200;
#else
  return false;
#endif
}

bool WhipClient::Unpublish(const std::string& participant_id) {
#if defined(WEBRTC_WIN)
  std::string url = config_.server_url + participant_id;
  std::string response_body;

  RTC_LOG(LS_INFO) << "WHIP: DELETE " << url;

  int status = HttpRequest("DELETE", url, "", "", config_.bearer_token, "",
                           &response_body, nullptr);
  return status == 200 || status == 204;
#else
  return false;
#endif
}

}  // namespace hublive
