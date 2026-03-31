/**
 * HeartbeatLine — SVG animated status indicator.
 * Ported from Veyon, adapted for HubLive design tokens.
 */

type HeartbeatState =
  | "online"
  | "connected"
  | "connecting"
  | "disconnecting"
  | "offline"
  | "locked"
  | "unknown";

interface HeartbeatLineProps {
  state: HeartbeatState;
  width?: number;
  height?: number;
}

const CONFIG: Record<string, { color: string; zigzag: boolean; duration: string }> = {
  online:        { color: "var(--success)",        zigzag: true,  duration: "1.5s" },
  connected:     { color: "var(--accent)",         zigzag: true,  duration: "1.2s" },
  connecting:    { color: "var(--warning)",         zigzag: true,  duration: "1s"   },
  disconnecting: { color: "var(--warning)",         zigzag: true,  duration: "2s"   },
  offline:       { color: "var(--bg-tertiary)",     zigzag: false, duration: "0s"   },
  locked:        { color: "var(--danger)",          zigzag: false, duration: "0s"   },
  unknown:       { color: "var(--bg-tertiary)",     zigzag: false, duration: "0s"   },
};

export function HeartbeatLine({ state, width = 32, height = 12 }: HeartbeatLineProps) {
  const { color, zigzag, duration } = CONFIG[state] ?? CONFIG.unknown!;
  const mid = height / 2;

  const zigzagPoints = zigzag
    ? [
        `0,${mid}`,
        `${width * 0.15},${mid}`,
        `${width * 0.23},${mid - mid * 0.85}`,
        `${width * 0.31},${mid + mid * 0.85}`,
        `${width * 0.39},${mid - mid * 0.5}`,
        `${width * 0.47},${mid + mid * 0.5}`,
        `${width * 0.54},${mid - mid * 0.2}`,
        `${width * 0.60},${mid + mid * 0.2}`,
        `${width * 0.68},${mid}`,
        `${width},${mid}`,
      ].join(" ")
    : `0,${mid} ${width},${mid}`;

  const pathLength = zigzag ? width * 2.5 : width;

  return (
    <svg
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      fill="none"
      className="shrink-0"
    >
      <polyline
        points={zigzagPoints}
        stroke={color}
        strokeWidth={1.5}
        strokeLinecap="round"
        strokeLinejoin="round"
        fill="none"
        strokeDasharray={zigzag ? pathLength : undefined}
        strokeDashoffset={zigzag ? pathLength : undefined}
      >
        {zigzag && (
          <animate
            attributeName="stroke-dashoffset"
            from={String(pathLength)}
            to="0"
            dur={duration}
            repeatCount="indefinite"
          />
        )}
      </polyline>
    </svg>
  );
}
