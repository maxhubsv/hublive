#include "input_injector.h"
#include <cstdio>
#include <algorithm>

void InputInjector::Init(int screen_width, int screen_height) {
    screen_width_ = screen_width;
    screen_height_ = screen_height;
    last_mouse_seq_ = 0;
    last_kb_seq_ = 0;
    printf("[InputInjector] Init: %dx%d\n", screen_width_, screen_height_);
}

// ---------------------------------------------------------------------------
// Low-level SendInput wrappers
// ---------------------------------------------------------------------------

void InputInjector::SendMouseInput(int abs_x, int abs_y, DWORD flags, DWORD mouse_data) {
    INPUT input = {};
    input.type = INPUT_MOUSE;
    input.mi.dx = abs_x;
    input.mi.dy = abs_y;
    input.mi.dwFlags = flags | MOUSEEVENTF_ABSOLUTE;
    input.mi.mouseData = mouse_data;
    input.mi.time = 0;
    input.mi.dwExtraInfo = 0;
    SendInput(1, &input, sizeof(INPUT));
}

void InputInjector::SendKeyInput(WORD vk, bool down) {
    INPUT input = {};
    input.type = INPUT_KEYBOARD;
    input.ki.wVk = vk;
    input.ki.wScan = static_cast<WORD>(MapVirtualKey(vk, MAPVK_VK_TO_VSC));
    input.ki.dwFlags = down ? 0 : KEYEVENTF_KEYUP;

    // Extended keys: arrows, Insert, Delete, Home, End, PageUp, PageDown,
    // NumLock, right Ctrl/Alt, Enter on numpad, etc.
    if (vk == VK_UP || vk == VK_DOWN || vk == VK_LEFT || vk == VK_RIGHT ||
        vk == VK_INSERT || vk == VK_DELETE || vk == VK_HOME || vk == VK_END ||
        vk == VK_PRIOR || vk == VK_NEXT || vk == VK_RCONTROL || vk == VK_RMENU ||
        vk == VK_LWIN || vk == VK_RWIN || vk == VK_NUMLOCK) {
        input.ki.dwFlags |= KEYEVENTF_EXTENDEDKEY;
    }

    SendInput(1, &input, sizeof(INPUT));
}

// ---------------------------------------------------------------------------
// JS keyCode -> Win32 Virtual Key mapping
// ---------------------------------------------------------------------------

WORD InputInjector::JsKeyCodeToVirtualKey(int js_key_code) {
    // JS keyCode values map directly to VK codes for most keys.

    // Letters A-Z: JS 65-90 == VK_A - VK_Z (0x41-0x5A)
    if (js_key_code >= 65 && js_key_code <= 90) {
        return static_cast<WORD>(js_key_code);
    }

    // Digits 0-9: JS 48-57 == VK_0 - VK_9 (0x30-0x39)
    if (js_key_code >= 48 && js_key_code <= 57) {
        return static_cast<WORD>(js_key_code);
    }

    // F1-F12: JS 112-123 == VK_F1 - VK_F12 (0x70-0x7B)
    if (js_key_code >= 112 && js_key_code <= 123) {
        return static_cast<WORD>(js_key_code);
    }

    // Numpad 0-9: JS 96-105 == VK_NUMPAD0 - VK_NUMPAD9 (0x60-0x69)
    if (js_key_code >= 96 && js_key_code <= 105) {
        return static_cast<WORD>(js_key_code);
    }

    switch (js_key_code) {
        case 8:   return VK_BACK;
        case 9:   return VK_TAB;
        case 13:  return VK_RETURN;
        case 16:  return VK_SHIFT;
        case 17:  return VK_CONTROL;
        case 18:  return VK_MENU;       // Alt
        case 19:  return VK_PAUSE;
        case 20:  return VK_CAPITAL;     // Caps Lock
        case 27:  return VK_ESCAPE;
        case 32:  return VK_SPACE;
        case 33:  return VK_PRIOR;       // Page Up
        case 34:  return VK_NEXT;        // Page Down
        case 35:  return VK_END;
        case 36:  return VK_HOME;
        case 37:  return VK_LEFT;
        case 38:  return VK_UP;
        case 39:  return VK_RIGHT;
        case 40:  return VK_DOWN;
        case 44:  return VK_SNAPSHOT;    // Print Screen
        case 45:  return VK_INSERT;
        case 46:  return VK_DELETE;
        case 91:  return VK_LWIN;        // Meta / Windows key
        case 92:  return VK_RWIN;
        case 93:  return VK_APPS;        // Context menu

        // Numpad operators
        case 106: return VK_MULTIPLY;
        case 107: return VK_ADD;
        case 109: return VK_SUBTRACT;
        case 110: return VK_DECIMAL;
        case 111: return VK_DIVIDE;
        case 144: return VK_NUMLOCK;
        case 145: return VK_SCROLL;

        // Punctuation (US layout)
        case 186: return VK_OEM_1;       // ;:
        case 187: return VK_OEM_PLUS;    // =+
        case 188: return VK_OEM_COMMA;   // ,<
        case 189: return VK_OEM_MINUS;   // -_
        case 190: return VK_OEM_PERIOD;  // .>
        case 191: return VK_OEM_2;       // /?
        case 192: return VK_OEM_3;       // `~
        case 219: return VK_OEM_4;       // [{
        case 220: return VK_OEM_5;       // backslash |
        case 221: return VK_OEM_6;       // ]}
        case 222: return VK_OEM_7;       // '"

        default:
            // Many JS keyCodes match Win32 VK codes directly.
            if (js_key_code > 0 && js_key_code < 256)
                return static_cast<WORD>(js_key_code);
            return 0;
    }
}

// ---------------------------------------------------------------------------
// High-level injection methods
// ---------------------------------------------------------------------------

void InputInjector::InjectMouseMove(float norm_x, float norm_y) {
    norm_x = std::clamp(norm_x, 0.0f, 1.0f);
    norm_y = std::clamp(norm_y, 0.0f, 1.0f);
    int abs_x = static_cast<int>(norm_x * 65535.0f);
    int abs_y = static_cast<int>(norm_y * 65535.0f);
    SendMouseInput(abs_x, abs_y, MOUSEEVENTF_MOVE);
}

void InputInjector::InjectMouseButton(float norm_x, float norm_y, int button, bool down) {
    norm_x = std::clamp(norm_x, 0.0f, 1.0f);
    norm_y = std::clamp(norm_y, 0.0f, 1.0f);
    int abs_x = static_cast<int>(norm_x * 65535.0f);
    int abs_y = static_cast<int>(norm_y * 65535.0f);

    DWORD flags = MOUSEEVENTF_MOVE;  // Move to position first
    DWORD mouse_data = 0;

    switch (button) {
        case 0:  // Left
            flags |= down ? MOUSEEVENTF_LEFTDOWN : MOUSEEVENTF_LEFTUP;
            break;
        case 1:  // Middle
            flags |= down ? MOUSEEVENTF_MIDDLEDOWN : MOUSEEVENTF_MIDDLEUP;
            break;
        case 2:  // Right
            flags |= down ? MOUSEEVENTF_RIGHTDOWN : MOUSEEVENTF_RIGHTUP;
            break;
        case 3:  // X1 (back)
            flags |= down ? MOUSEEVENTF_XDOWN : MOUSEEVENTF_XUP;
            mouse_data = XBUTTON1;
            break;
        case 4:  // X2 (forward)
            flags |= down ? MOUSEEVENTF_XDOWN : MOUSEEVENTF_XUP;
            mouse_data = XBUTTON2;
            break;
        default:
            return;
    }

    SendMouseInput(abs_x, abs_y, flags, mouse_data);
}

void InputInjector::InjectMouseScroll(float norm_x, float norm_y, int delta) {
    norm_x = std::clamp(norm_x, 0.0f, 1.0f);
    norm_y = std::clamp(norm_y, 0.0f, 1.0f);
    int abs_x = static_cast<int>(norm_x * 65535.0f);
    int abs_y = static_cast<int>(norm_y * 65535.0f);

    // Move to position, then scroll. delta is in units of WHEEL_DELTA (120).
    SendMouseInput(abs_x, abs_y, MOUSEEVENTF_MOVE | MOUSEEVENTF_WHEEL,
                   static_cast<DWORD>(delta));
}

void InputInjector::InjectKeyboard(int key_code, bool down, int modifiers) {
    WORD vk = JsKeyCodeToVirtualKey(key_code);
    if (vk == 0) return;

    // If the key itself is a modifier key, just send it directly.
    if (vk == VK_CONTROL || vk == VK_SHIFT || vk == VK_MENU || vk == VK_LWIN || vk == VK_RWIN) {
        SendKeyInput(vk, down);
        return;
    }

    // For non-modifier keys: press required modifiers before key down,
    // release them after key up.
    bool need_ctrl  = (modifiers & 1) != 0;
    bool need_shift = (modifiers & 2) != 0;
    bool need_alt   = (modifiers & 4) != 0;

    if (down) {
        // Press modifiers first
        if (need_ctrl)  SendKeyInput(VK_CONTROL, true);
        if (need_shift) SendKeyInput(VK_SHIFT, true);
        if (need_alt)   SendKeyInput(VK_MENU, true);
        SendKeyInput(vk, true);
    } else {
        SendKeyInput(vk, false);
        // Release modifiers after
        if (need_alt)   SendKeyInput(VK_MENU, false);
        if (need_shift) SendKeyInput(VK_SHIFT, false);
        if (need_ctrl)  SendKeyInput(VK_CONTROL, false);
    }
}

// ---------------------------------------------------------------------------
// Message router
// ---------------------------------------------------------------------------

bool InputInjector::ProcessMessage(int type, uint32_t seq, int action,
                                   float x, float y, int button, int delta,
                                   int key_code, int modifiers) {
    if (type == 1) {
        // Mouse event — drop out-of-order for move events only.
        if (action == 1 && seq != 0 && seq < last_mouse_seq_) {
            return false;  // Stale mouse move, drop it
        }
        if (seq != 0) last_mouse_seq_ = seq;

        switch (action) {
            case 1:  // move
                InjectMouseMove(x, y);
                break;
            case 2:  // down
                InjectMouseButton(x, y, button, true);
                break;
            case 3:  // up
                InjectMouseButton(x, y, button, false);
                break;
            case 4:  // scroll
                InjectMouseScroll(x, y, delta);
                break;
            default:
                return false;
        }
        return true;

    } else if (type == 2) {
        // Keyboard event — never drop (all key events are important).
        if (seq != 0) last_kb_seq_ = seq;

        switch (action) {
            case 1:  // key down
                InjectKeyboard(key_code, true, modifiers);
                break;
            case 2:  // key up
                InjectKeyboard(key_code, false, modifiers);
                break;
            default:
                return false;
        }
        return true;
    }

    return false;
}
