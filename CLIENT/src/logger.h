#pragma once

#include <cstdio>
#include <ctime>
#include <cstdarg>
#include <string>
#include <mutex>

enum LogLevel { LOG_LVL_DEBUG = 0, LOG_LVL_INFO = 1, LOG_LVL_WARN = 2, LOG_LVL_ERROR = 3, LOG_LVL_NONE = 4 };

class Logger {
public:
    static Logger& Instance() {
        static Logger instance;
        return instance;
    }

    void SetLevel(int level) { level_ = level; }
    void SetLevel(const std::string& level) {
        if (level == "debug") level_ = LOG_LVL_DEBUG;
        else if (level == "info") level_ = LOG_LVL_INFO;
        else if (level == "warn") level_ = LOG_LVL_WARN;
        else if (level == "error") level_ = LOG_LVL_ERROR;
        else if (level == "none") level_ = LOG_LVL_NONE;
    }

    void SetFileOutput(const std::string& path) {
        std::lock_guard<std::mutex> lock(mutex_);
        if (file_) fclose(file_);
        file_ = fopen(path.c_str(), "a");
    }

    void Write(int level, const char* module, const char* fmt, ...) {
        if (level < level_) return;

        std::lock_guard<std::mutex> lock(mutex_);

        time_t now = time(nullptr);
        struct tm tm_buf;
        localtime_s(&tm_buf, &now);
        char time_str[32];
        strftime(time_str, sizeof(time_str), "%Y-%m-%d %H:%M:%S", &tm_buf);

        const char* lvl = "";
        switch (level) {
            case LOG_LVL_DEBUG: lvl = "DEBUG"; break;
            case LOG_LVL_INFO:  lvl = "INFO "; break;
            case LOG_LVL_WARN:  lvl = "WARN "; break;
            case LOG_LVL_ERROR: lvl = "ERROR"; break;
            default: break;
        }

        char msg[1024];
        va_list args;
        va_start(args, fmt);
        vsnprintf(msg, sizeof(msg), fmt, args);
        va_end(args);

        fprintf(stdout, "[%s] [%s] [%s] %s\n", time_str, lvl, module, msg);
        fflush(stdout);

        if (file_) {
            fprintf(file_, "[%s] [%s] [%s] %s\n", time_str, lvl, module, msg);
            fflush(file_);
        }
    }

    ~Logger() {
        if (file_) fclose(file_);
    }

private:
    Logger() = default;
    int level_ = LOG_LVL_INFO;
    FILE* file_ = nullptr;
    std::mutex mutex_;
};

// Inline log functions — no macros, no conflicts
inline void LogDebug(const char* m, const char* f, ...) { va_list a; va_start(a, f); char b[1024]; vsnprintf(b, sizeof(b), f, a); va_end(a); Logger::Instance().Write(LOG_LVL_DEBUG, m, "%s", b); }
inline void LogInfo(const char* m, const char* f, ...)   { va_list a; va_start(a, f); char b[1024]; vsnprintf(b, sizeof(b), f, a); va_end(a); Logger::Instance().Write(LOG_LVL_INFO,  m, "%s", b); }
inline void LogWarn(const char* m, const char* f, ...)   { va_list a; va_start(a, f); char b[1024]; vsnprintf(b, sizeof(b), f, a); va_end(a); Logger::Instance().Write(LOG_LVL_WARN,  m, "%s", b); }
inline void LogError(const char* m, const char* f, ...)  { va_list a; va_start(a, f); char b[1024]; vsnprintf(b, sizeof(b), f, a); va_end(a); Logger::Instance().Write(LOG_LVL_ERROR, m, "%s", b); }

// Short aliases
#define LOG_D LogDebug
#define LOG_I LogInfo
#define LOG_W LogWarn
#define LOG_E LogError
