#pragma once

#include <windows.h>
#include <cstdint>

class InputInjector {
public:
    void Init(int screen_width, int screen_height);

    // Mouse events with normalized coordinates (0.0 - 1.0).
    void InjectMouseMove(float norm_x, float norm_y);
    void InjectMouseButton(float norm_x, float norm_y, int button, bool down);
    void InjectMouseScroll(float norm_x, float norm_y, int delta);

    // Keyboard event. key_code is JS keyCode, modifiers is bitmask:
    // 1=Ctrl, 2=Shift, 4=Alt, 8=Meta.
    void InjectKeyboard(int key_code, bool down, int modifiers);

    // Process a deserialized input message. Returns false if the message
    // was dropped (e.g. out-of-order or unrecognized type).
    // Fields: t=type(1=mouse,2=keyboard), s=sequence, a=action,
    //   x,y=normalized coords, b=button, d=scroll delta,
    //   k=key code, m=modifier bitmask.
    bool ProcessMessage(int type, uint32_t seq, int action,
                        float x, float y, int button, int delta,
                        int key_code, int modifiers);

private:
    int screen_width_ = 1920;
    int screen_height_ = 1080;
    uint32_t last_mouse_seq_ = 0;
    uint32_t last_kb_seq_ = 0;

    void SendMouseInput(int abs_x, int abs_y, DWORD flags, DWORD mouse_data = 0);
    void SendKeyInput(WORD vk, bool down);
    WORD JsKeyCodeToVirtualKey(int js_key_code);
};
