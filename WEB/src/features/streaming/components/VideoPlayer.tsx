import { useTranslation } from "react-i18next";
import { cn } from "@/shared/utils/cn";
import { Spinner } from "@/shared/ui/spinner";
import type { ConnectionState } from "../types/streaming.types";

interface VideoPlayerProps {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  connectionState: ConnectionState;
  controlActive: boolean;
  onActivate: () => void;
  handlers: {
    onMouseMove: (e: React.MouseEvent) => void;
    onMouseDown: (e: React.MouseEvent) => void;
    onMouseUp: (e: React.MouseEvent) => void;
    onWheel: (e: React.WheelEvent) => void;
    onKeyDown: (e: React.KeyboardEvent) => void;
    onKeyUp: (e: React.KeyboardEvent) => void;
    onBlur: () => void;
    onContextMenu: (e: React.MouseEvent) => void;
  };
}

const STATUS_STYLES: Record<ConnectionState, string> = {
  idle: "bg-text-secondary",
  connecting: "bg-warning",
  connected: "bg-success",
  disconnected: "bg-danger",
  error: "bg-danger",
};

export function VideoPlayer({
  videoRef,
  connectionState,
  controlActive,
  onActivate,
  handlers,
}: VideoPlayerProps) {
  const { t } = useTranslation();

  const statusKey =
    connectionState === "idle"
      ? "stream.disconnected"
      : `stream.${connectionState}`;

  return (
    <div
      tabIndex={0}
      className={cn(
        "relative h-full overflow-hidden rounded-lg border-2 bg-black outline-none transition-colors",
        controlActive
          ? "border-success cursor-none"
          : "border-bg-tertiary cursor-pointer",
      )}
      onClick={!controlActive ? onActivate : undefined}
      onMouseMove={handlers.onMouseMove}
      onMouseDown={handlers.onMouseDown}
      onMouseUp={handlers.onMouseUp}
      onWheel={handlers.onWheel}
      onKeyDown={handlers.onKeyDown}
      onKeyUp={handlers.onKeyUp}
      onBlur={handlers.onBlur}
      onContextMenu={handlers.onContextMenu}
    >
      {/* Status badge */}
      <div className="absolute left-tight top-tight z-10 flex items-center gap-element rounded-full bg-bg-primary/80 px-tight py-micro backdrop-blur-sm">
        <span
          className={cn(
            "size-dot rounded-full",
            STATUS_STYLES[connectionState],
            connectionState === "connecting" && "animate-pulse",
          )}
        />
        <span className="text-caption font-medium text-text-secondary">
          {t(statusKey)}
        </span>
      </div>

      {/* Video element */}
      <video
        ref={videoRef}
        autoPlay
        playsInline
        muted
        className="h-full w-full object-contain"
      />

      {/* Click-to-activate hint */}
      {connectionState === "connected" && !controlActive && (
        <div className="absolute inset-0 flex items-center justify-center bg-black/40 transition-opacity">
          <div className="rounded-lg bg-bg-primary/90 px-page py-tight text-center backdrop-blur-sm">
            <p className="text-body font-medium text-text-primary">
              {t("stream.clickToControl")}
            </p>
            <p className="mt-hairline text-caption text-text-secondary">
              {t("stream.pressEscapeToRelease")}
            </p>
          </div>
        </div>
      )}

      {/* No stream overlay */}
      {connectionState !== "connected" && connectionState !== "connecting" && (
        <div className="absolute inset-0 flex items-center justify-center bg-bg-primary/60">
          <p className="text-text-secondary">{t("stream.noStream")}</p>
        </div>
      )}

      {/* Connecting spinner */}
      {connectionState === "connecting" && (
        <div className="absolute inset-0 flex items-center justify-center bg-bg-primary/60">
          <Spinner />
        </div>
      )}
    </div>
  );
}
