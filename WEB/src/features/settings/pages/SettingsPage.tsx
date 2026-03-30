import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  Globe,
  Palette,
  Wifi,
  MonitorPlay,
  Zap,
  Info,
  Cpu,
  Code,
} from "lucide-react";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { ToggleSwitch } from "@/shared/ui/toggle-switch";
import { SettingSection } from "../components/SettingSection";
import { SettingItem } from "../components/SettingItem";
import { useSettingsStore } from "../store/settings.store";
import { LOCALES, LOCALE_LABELS, type Locale } from "@/shared/constants";

export default function SettingsPage() {
  const { t, i18n } = useTranslation();
  const {
    livekitUrl,
    roomName,
    autoConnect,
    setLivekitUrl,
    setRoomName,
    setAutoConnect,
  } = useSettingsStore();

  const [urlDraft, setUrlDraft] = useState(livekitUrl);
  const [roomDraft, setRoomDraft] = useState(roomName);

  const urlChanged = urlDraft !== livekitUrl;
  const roomChanged = roomDraft !== roomName;

  const saveUrl = () => setLivekitUrl(urlDraft);
  const saveRoom = () => setRoomName(roomDraft);

  return (
    <div className="flex h-full flex-col gap-section overflow-hidden">
      <div className="shrink-0">
        <h1 className="text-page-title font-bold">{t("settings.title")}</h1>
        <p className="mt-tight text-text-secondary">{t("settings.subtitle")}</p>
      </div>

      <div className="flex-1 space-y-section overflow-y-auto pr-element">
        {/* ── Appearance ── */}
        <SettingSection title={t("settings.appearance")}>
          <SettingItem
            icon={Globe}
            title={t("settings.language")}
            description={t("settings.languageDesc")}
            control={
              <div className="flex gap-hairline">
                {LOCALES.map((locale) => (
                  <Button
                    key={locale}
                    variant={i18n.language === locale ? "default" : "ghost"}
                    size="sm"
                    onClick={() => i18n.changeLanguage(locale)}
                  >
                    {LOCALE_LABELS[locale as Locale]}
                  </Button>
                ))}
              </div>
            }
          />
          <SettingItem
            icon={Palette}
            title={t("settings.theme")}
            description={t("settings.themeDesc")}
            isLast
            control={
              <Button variant="secondary" size="sm" disabled>
                {t("settings.dark")}
              </Button>
            }
          />
        </SettingSection>

        {/* ── Streaming ── */}
        <SettingSection title={t("settings.streaming")}>
          <SettingItem
            icon={Wifi}
            title={t("settings.serverUrl")}
            description={t("settings.serverUrlDesc")}
            control={
              <div className="flex items-center gap-element">
                <Input
                  value={urlDraft}
                  onChange={(e) => setUrlDraft(e.target.value)}
                  className="w-64"
                />
                {urlChanged && (
                  <Button size="sm" onClick={saveUrl}>
                    {t("settings.save")}
                  </Button>
                )}
              </div>
            }
          />
          <SettingItem
            icon={MonitorPlay}
            title={t("settings.roomName")}
            description={t("settings.roomNameDesc")}
            control={
              <div className="flex items-center gap-element">
                <Input
                  value={roomDraft}
                  onChange={(e) => setRoomDraft(e.target.value)}
                  className="w-48"
                />
                {roomChanged && (
                  <Button size="sm" onClick={saveRoom}>
                    {t("settings.save")}
                  </Button>
                )}
              </div>
            }
          />
          <SettingItem
            icon={Zap}
            title={t("settings.autoConnect")}
            description={t("settings.autoConnectDesc")}
            isLast
            control={
              <ToggleSwitch
                checked={autoConnect}
                onChange={setAutoConnect}
              />
            }
          />
        </SettingSection>

        {/* ── About ── */}
        <SettingSection title={t("settings.about")}>
          <SettingItem
            icon={Info}
            title="HubLive"
            description={t("settings.aboutDesc")}
          />
          <SettingItem
            icon={Code}
            title={t("settings.version")}
            description="0.1.0"
          />
          <SettingItem
            icon={Cpu}
            title={t("settings.techStack")}
            description="React 19 · Vite · TypeScript · Tailwind v4 · LiveKit"
            isLast
          />
        </SettingSection>
      </div>
    </div>
  );
}
