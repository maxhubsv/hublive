export const APP_NAME = "HubLive";

export const DEFAULT_LIVEKIT_URL =
  import.meta.env.VITE_LIVEKIT_URL || "ws://103.89.94.96:7880";

export const DEFAULT_ROOM_NAME = "screen-share";

/** Dev-mode credentials (--dev flag on server) */
export const LIVEKIT_API_KEY = "devkey";
export const LIVEKIT_API_SECRET = "secret";

export const VIEWER_IDENTITY = `web-viewer-${Math.random().toString(36).slice(2, 6)}`;

/** Mouse move throttle interval (ms) — ~60fps */
export const MOUSE_THROTTLE_MS = 16;

export const LOCALES = ["vi", "en", "zh"] as const;
export type Locale = (typeof LOCALES)[number];

export const LOCALE_LABELS: Record<Locale, string> = {
  vi: "Tiếng Việt",
  en: "English",
  zh: "中文",
};
