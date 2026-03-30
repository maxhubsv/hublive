export type ConnectionState =
  | "idle"
  | "connecting"
  | "connected"
  | "disconnected"
  | "error";

// --- Input Event Protocol (matches C++ agent) ---

/** Mouse action codes */
export const MouseAction = {
  Move: 1,
  Down: 2,
  Up: 3,
  Wheel: 4,
} as const;

/** Keyboard action codes */
export const KeyAction = {
  Down: 1,
  Up: 2,
} as const;

/** Modifier bitmask */
export const Modifier = {
  Ctrl: 1,
  Shift: 2,
  Alt: 4,
  Meta: 8,
} as const;

export interface MouseInputEvent {
  t: 1;
  s: number;
  a: (typeof MouseAction)[keyof typeof MouseAction];
  x: number;
  y: number;
  b: number;
  d: number;
}

export interface KeyboardInputEvent {
  t: 2;
  s: number;
  a: (typeof KeyAction)[keyof typeof KeyAction];
  k: string;
  m: number;
}

export type InputEvent = MouseInputEvent | KeyboardInputEvent;
