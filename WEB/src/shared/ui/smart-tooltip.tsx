/**
 * SmartTooltip — Auto-positioning portal tooltip with arrow.
 * Ported from Veyon, adapted for HubLive design tokens.
 */

import {
  useState,
  useRef,
  useCallback,
  useEffect,
  type ReactNode,
} from "react";
import { createPortal } from "react-dom";

type Position = "auto" | "top" | "bottom" | "left" | "right";

interface SmartTooltipProps {
  content: ReactNode;
  children: ReactNode;
  delay?: number;
  position?: Position;
  interactive?: boolean;
}

interface TooltipPos {
  top: number;
  left: number;
  placement: "top" | "bottom" | "left" | "right";
}

const ARROW_SIZE = 6;
const GAP = 4;

function computePosition(
  triggerRect: DOMRect,
  tooltipEl: HTMLDivElement,
  preferred: Position,
): TooltipPos {
  const tw = tooltipEl.offsetWidth;
  const th = tooltipEl.offsetHeight;
  const vw = window.innerWidth;
  const vh = window.innerHeight;

  const spaceTop = triggerRect.top;
  const spaceBottom = vh - triggerRect.bottom;
  const spaceRight = vw - triggerRect.right;

  let placement: "top" | "bottom" | "left" | "right";

  if (preferred !== "auto") {
    placement = preferred;
  } else {
    if (spaceTop >= th + ARROW_SIZE + GAP) placement = "top";
    else if (spaceBottom >= th + ARROW_SIZE + GAP) placement = "bottom";
    else if (spaceRight >= tw + ARROW_SIZE + GAP) placement = "right";
    else placement = "left";
  }

  let top = 0;
  let left = 0;
  const cx = triggerRect.left + triggerRect.width / 2;
  const cy = triggerRect.top + triggerRect.height / 2;

  switch (placement) {
    case "top":    top = triggerRect.top - th - ARROW_SIZE - GAP; left = cx - tw / 2; break;
    case "bottom": top = triggerRect.bottom + ARROW_SIZE + GAP;   left = cx - tw / 2; break;
    case "left":   top = cy - th / 2; left = triggerRect.left - tw - ARROW_SIZE - GAP; break;
    case "right":  top = cy - th / 2; left = triggerRect.right + ARROW_SIZE + GAP; break;
  }

  left = Math.max(4, Math.min(left, vw - tw - 4));
  top = Math.max(4, Math.min(top, vh - th - 4));
  return { top, left, placement };
}

function Arrow({ placement }: { placement: "top" | "bottom" | "left" | "right" }) {
  const s = ARROW_SIZE;
  const base: React.CSSProperties = { position: "absolute", width: 0, height: 0 };

  const outer: Record<string, React.CSSProperties> = {
    top:    { ...base, bottom: -s, left: "50%", transform: "translateX(-50%)", borderLeft: `${s}px solid transparent`, borderRight: `${s}px solid transparent`, borderTop: `${s}px solid var(--bg-tertiary)` },
    bottom: { ...base, top: -s, left: "50%", transform: "translateX(-50%)", borderLeft: `${s}px solid transparent`, borderRight: `${s}px solid transparent`, borderBottom: `${s}px solid var(--bg-tertiary)` },
    left:   { ...base, right: -s, top: "50%", transform: "translateY(-50%)", borderTop: `${s}px solid transparent`, borderBottom: `${s}px solid transparent`, borderLeft: `${s}px solid var(--bg-tertiary)` },
    right:  { ...base, left: -s, top: "50%", transform: "translateY(-50%)", borderTop: `${s}px solid transparent`, borderBottom: `${s}px solid transparent`, borderRight: `${s}px solid var(--bg-tertiary)` },
  };
  const inner: Record<string, React.CSSProperties> = {
    top:    { position: "absolute", bottom: 1, left: -s, width: 0, height: 0, borderLeft: `${s}px solid transparent`, borderRight: `${s}px solid transparent`, borderTop: `${s}px solid var(--bg-secondary)` },
    bottom: { position: "absolute", top: 1, left: -s, width: 0, height: 0, borderLeft: `${s}px solid transparent`, borderRight: `${s}px solid transparent`, borderBottom: `${s}px solid var(--bg-secondary)` },
    left:   { position: "absolute", right: 1, top: -s, width: 0, height: 0, borderTop: `${s}px solid transparent`, borderBottom: `${s}px solid transparent`, borderLeft: `${s}px solid var(--bg-secondary)` },
    right:  { position: "absolute", left: 1, top: -s, width: 0, height: 0, borderTop: `${s}px solid transparent`, borderBottom: `${s}px solid transparent`, borderRight: `${s}px solid var(--bg-secondary)` },
  };
  return <div style={outer[placement]}><div style={inner[placement]} /></div>;
}

const TRANSLATE_MAP: Record<string, string> = {
  top: "translateY(4px)", bottom: "translateY(-4px)",
  left: "translateX(4px)", right: "translateX(-4px)",
};

export function SmartTooltip({
  content,
  children,
  delay = 100,
  position = "auto",
  interactive = false,
}: SmartTooltipProps) {
  const [visible, setVisible] = useState(false);
  const [pos, setPos] = useState<TooltipPos | null>(null);
  const [animating, setAnimating] = useState(false);
  const childRef = useRef<HTMLElement | null>(null);
  const wrapperRef = useRef<HTMLSpanElement>(null);
  const tooltipRef = useRef<HTMLDivElement>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout>>(undefined);

  const doShow = useCallback(() => {
    timerRef.current = setTimeout(() => {
      setVisible(true);
      setAnimating(true);
    }, delay);
  }, [delay]);

  const doHide = useCallback(() => {
    clearTimeout(timerRef.current);
    setAnimating(false);
    setTimeout(() => setVisible(false), 100);
  }, []);

  const doHideDelayed = useCallback(() => {
    clearTimeout(timerRef.current);
    timerRef.current = setTimeout(doHide, 150);
  }, [doHide]);

  useEffect(() => {
    const wrapper = wrapperRef.current;
    if (!wrapper) return;
    const el = wrapper.firstElementChild as HTMLElement | null;
    if (!el) return;
    childRef.current = el;

    const onEnter = () => doShow();
    const onLeave = () => (interactive ? doHideDelayed() : doHide());

    el.addEventListener("mouseenter", onEnter);
    el.addEventListener("mouseleave", onLeave);

    return () => {
      el.removeEventListener("mouseenter", onEnter);
      el.removeEventListener("mouseleave", onLeave);
    };
  }, [doShow, doHide, doHideDelayed, interactive, children]);

  useEffect(() => {
    return () => clearTimeout(timerRef.current);
  }, []);

  useEffect(() => {
    if (visible && childRef.current && tooltipRef.current) {
      const rect = childRef.current.getBoundingClientRect();
      setPos(computePosition(rect, tooltipRef.current, position));
    }
  }, [visible, position]);

  return (
    <>
      <span ref={wrapperRef} style={{ display: "contents" }}>
        {children}
      </span>
      {visible &&
        createPortal(
          <div
            ref={tooltipRef}
            onMouseEnter={interactive ? () => clearTimeout(timerRef.current) : undefined}
            onMouseLeave={interactive ? doHide : undefined}
            style={{
              position: "fixed",
              top: pos?.top ?? -9999,
              left: pos?.left ?? -9999,
              zIndex: 9999,
              maxWidth: interactive ? 320 : 240,
              padding: "6px 12px",
              borderRadius: 8,
              border: "1px solid var(--bg-tertiary)",
              background: "var(--bg-secondary)",
              boxShadow: "0 4px 16px rgba(0,0,0,0.4), 0 1px 4px rgba(0,0,0,0.2)",
              color: "var(--text-primary)",
              fontSize: "var(--text-caption)",
              lineHeight: 1.4,
              wordWrap: "break-word",
              pointerEvents: interactive ? "auto" : "none",
              opacity: animating ? 1 : 0,
              transform: animating ? "translate(0)" : (pos ? TRANSLATE_MAP[pos.placement] : "translateY(4px)"),
              transition: animating
                ? "opacity 150ms ease-out, transform 150ms ease-out"
                : "opacity 100ms ease-in, transform 100ms ease-in",
            }}
          >
            {content}
            {pos && <Arrow placement={pos.placement} />}
          </div>,
          document.body,
        )}
    </>
  );
}
