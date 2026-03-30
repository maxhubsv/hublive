import { useCallback, useRef, useState } from "react";
import {
  MouseAction,
  KeyAction,
  type InputEvent,
} from "../types/streaming.types";
import { MOUSE_THROTTLE_MS } from "@/shared/constants";

interface UseInputControlOptions {
  sendInput: (event: InputEvent, reliable: boolean) => void;
  videoRef: React.RefObject<HTMLVideoElement | null>;
}

function getModifiers(e: KeyboardEvent | MouseEvent): number {
  let m = 0;
  if (e.ctrlKey) m |= 1;
  if (e.shiftKey) m |= 2;
  if (e.altKey) m |= 4;
  if (e.metaKey) m |= 8;
  return m;
}

function getNormalizedCoords(
  e: MouseEvent,
  videoEl: HTMLVideoElement,
): { x: number; y: number } {
  const rect = videoEl.getBoundingClientRect();
  return {
    x: Math.max(0, Math.min(1, (e.clientX - rect.left) / rect.width)),
    y: Math.max(0, Math.min(1, (e.clientY - rect.top) / rect.height)),
  };
}

export function useInputControl({ sendInput, videoRef }: UseInputControlOptions) {
  const [controlActive, setControlActive] = useState(false);
  const [mouseEnabled, setMouseEnabled] = useState(true);
  const [keyboardEnabled, setKeyboardEnabled] = useState(true);
  const seqRef = useRef(0);
  const lastMoveRef = useRef(0);

  const nextSeq = () => ++seqRef.current;

  // --- Activate / deactivate ---

  const activate = useCallback(() => {
    setControlActive(true);
  }, []);

  const deactivate = useCallback(() => {
    setControlActive(false);
  }, []);

  // --- Mouse handlers ---

  const onMouseMove = useCallback(
    (e: React.MouseEvent) => {
      if (!controlActive || !mouseEnabled) return;
      const vid = videoRef.current;
      if (!vid) return;

      const now = performance.now();
      if (now - lastMoveRef.current < MOUSE_THROTTLE_MS) return;
      lastMoveRef.current = now;

      const { x, y } = getNormalizedCoords(e.nativeEvent, vid);
      sendInput(
        { t: 1, s: nextSeq(), a: MouseAction.Move, x, y, b: 0, d: 0 },
        false, // lossy for moves
      );
    },
    [controlActive, mouseEnabled, sendInput, videoRef],
  );

  const onMouseDown = useCallback(
    (e: React.MouseEvent) => {
      if (!controlActive || !mouseEnabled) return;
      const vid = videoRef.current;
      if (!vid) return;
      e.preventDefault();

      const { x, y } = getNormalizedCoords(e.nativeEvent, vid);
      sendInput(
        {
          t: 1,
          s: nextSeq(),
          a: MouseAction.Down,
          x,
          y,
          b: e.button as 0 | 1 | 2 | 3 | 4,
          d: 0,
        },
        true,
      );
    },
    [controlActive, mouseEnabled, sendInput, videoRef],
  );

  const onMouseUp = useCallback(
    (e: React.MouseEvent) => {
      if (!controlActive || !mouseEnabled) return;
      const vid = videoRef.current;
      if (!vid) return;
      e.preventDefault();

      const { x, y } = getNormalizedCoords(e.nativeEvent, vid);
      sendInput(
        {
          t: 1,
          s: nextSeq(),
          a: MouseAction.Up,
          x,
          y,
          b: e.button as 0 | 1 | 2 | 3 | 4,
          d: 0,
        },
        true,
      );
    },
    [controlActive, mouseEnabled, sendInput, videoRef],
  );

  const onWheel = useCallback(
    (e: React.WheelEvent) => {
      if (!controlActive || !mouseEnabled) return;
      const vid = videoRef.current;
      if (!vid) return;
      e.preventDefault();

      const { x, y } = getNormalizedCoords(e.nativeEvent as unknown as MouseEvent, vid);
      sendInput(
        {
          t: 1,
          s: nextSeq(),
          a: MouseAction.Wheel,
          x,
          y,
          b: 0,
          d: e.deltaY > 0 ? 1 : -1,
        },
        true,
      );
    },
    [controlActive, mouseEnabled, sendInput, videoRef],
  );

  // --- Keyboard handlers ---

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (!controlActive) return;

      // Escape deactivates control
      if (e.key === "Escape") {
        deactivate();
        return;
      }

      // Let F11/F12 pass through (fullscreen / devtools)
      if (e.key === "F11" || e.key === "F12") return;

      if (!keyboardEnabled) return;
      e.preventDefault();

      sendInput(
        {
          t: 2,
          s: nextSeq(),
          a: KeyAction.Down,
          k: e.keyCode,
          m: getModifiers(e.nativeEvent),
        },
        true,
      );
    },
    [controlActive, keyboardEnabled, sendInput, deactivate],
  );

  const onKeyUp = useCallback(
    (e: React.KeyboardEvent) => {
      if (!controlActive || !keyboardEnabled) return;
      if (e.key === "F11" || e.key === "F12") return;
      e.preventDefault();

      sendInput(
        {
          t: 2,
          s: nextSeq(),
          a: KeyAction.Up,
          k: e.keyCode,
          m: getModifiers(e.nativeEvent),
        },
        true,
      );
    },
    [controlActive, keyboardEnabled, sendInput],
  );

  const onBlur = useCallback(() => {
    deactivate();
  }, [deactivate]);

  const onContextMenu = useCallback(
    (e: React.MouseEvent) => {
      if (controlActive) e.preventDefault();
    },
    [controlActive],
  );

  return {
    controlActive,
    mouseEnabled,
    keyboardEnabled,
    setMouseEnabled,
    setKeyboardEnabled,
    activate,
    deactivate,
    handlers: {
      onMouseMove,
      onMouseDown,
      onMouseUp,
      onWheel,
      onKeyDown,
      onKeyUp,
      onBlur,
      onContextMenu,
    },
  };
}
