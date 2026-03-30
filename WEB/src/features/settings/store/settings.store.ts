import { create } from "zustand";

interface SettingsState {
  livekitUrl: string;
  roomName: string;
  autoConnect: boolean;
  setLivekitUrl: (url: string) => void;
  setRoomName: (name: string) => void;
  setAutoConnect: (auto: boolean) => void;
}

export const useSettingsStore = create<SettingsState>((set) => ({
  livekitUrl: localStorage.getItem("settings_livekit_url") || "ws://localhost:7880",
  roomName: localStorage.getItem("settings_room_name") || "screen-share",
  autoConnect: localStorage.getItem("settings_auto_connect") !== "false",

  setLivekitUrl: (url) => {
    localStorage.setItem("settings_livekit_url", url);
    set({ livekitUrl: url });
  },

  setRoomName: (name) => {
    localStorage.setItem("settings_room_name", name);
    set({ roomName: name });
  },

  setAutoConnect: (auto) => {
    localStorage.setItem("settings_auto_connect", String(auto));
    set({ autoConnect: auto });
  },
}));
