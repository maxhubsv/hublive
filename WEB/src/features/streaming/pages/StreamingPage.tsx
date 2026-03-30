import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/shared/ui/button";
import { VideoPlayer } from "../components/VideoPlayer";
import { useRoom } from "../hooks/useRoom";
import { useInputControl } from "../hooks/useInputControl";

export default function StreamingPage() {
  const { t } = useTranslation();
  const { connectionState, videoRef, connect, disconnect, sendInput } = useRoom();
  const {
    controlActive,
    mouseEnabled,
    keyboardEnabled,
    setMouseEnabled,
    setKeyboardEnabled,
    activate,
    handlers,
  } = useInputControl({ sendInput, videoRef });

  // Auto-connect on mount
  useEffect(() => {
    connect();
    return () => disconnect();
  }, [connect, disconnect]);

  const isConnected = connectionState === "connected";

  return (
    <div className="flex h-full flex-col gap-tight overflow-hidden">
      {/* Header bar */}
      <div className="flex shrink-0 items-center justify-between">
        <h1 className="text-page-title font-bold">{t("stream.title")}</h1>

        <div className="flex items-center gap-tight">
          {/* Control toggles */}
          {isConnected && (
            <div className="flex items-center gap-tight text-body">
              <label className="flex cursor-pointer items-center gap-tight text-text-secondary">
                <input
                  type="checkbox"
                  checked={mouseEnabled}
                  onChange={(e) => setMouseEnabled(e.target.checked)}
                  className="accent-accent"
                />
                Mouse
              </label>
              <label className="flex cursor-pointer items-center gap-tight text-text-secondary">
                <input
                  type="checkbox"
                  checked={keyboardEnabled}
                  onChange={(e) => setKeyboardEnabled(e.target.checked)}
                  className="accent-accent"
                />
                Keyboard
              </label>
            </div>
          )}

          {/* Connect / Disconnect */}
          {connectionState === "idle" ||
          connectionState === "disconnected" ||
          connectionState === "error" ? (
            <Button size="sm" onClick={connect}>
              {t("stream.connect")}
            </Button>
          ) : isConnected ? (
            <Button size="sm" variant="destructive" onClick={disconnect}>
              {t("stream.disconnect")}
            </Button>
          ) : null}
        </div>
      </div>

      {/* Video area — fills remaining space */}
      <div className="min-h-0 flex-1">
        <VideoPlayer
          videoRef={videoRef}
          connectionState={connectionState}
          controlActive={controlActive}
          onActivate={activate}
          handlers={handlers}
        />
      </div>
    </div>
  );
}
