import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { StatusBadge } from "@/shared/ui/status-badge";
import { LOCALES, LOCALE_LABELS, type Locale } from "@/shared/constants";

const DEFAULT_COLORS = [
  { name: "bg-primary", cssVar: "--bg-primary", hex: "#0A0A0B" },
  { name: "bg-secondary", cssVar: "--bg-secondary", hex: "#141416" },
  { name: "bg-tertiary", cssVar: "--bg-tertiary", hex: "#1C1C1F" },
  { name: "accent", cssVar: "--accent", hex: "#3B82F6" },
  { name: "success", cssVar: "--success", hex: "#22C55E" },
  { name: "danger", cssVar: "--danger", hex: "#EF4444" },
  { name: "warning", cssVar: "--warning", hex: "#F59E0B" },
];

const NAV_LINKS = [
  { to: "/dashboard", label: "sidebar.dashboard" },
  { to: "/streaming", label: "sidebar.streaming" },
  { to: "/login", label: "auth.login" },
];

export default function TestPage() {
  const { t, i18n } = useTranslation();
  const [colors, setColors] = useState(() =>
    DEFAULT_COLORS.map((c) => ({ ...c })),
  );

  const handleColorChange = useCallback(
    (index: number, newHex: string) => {
      setColors((prev) => {
        const next = [...prev];
        next[index] = { ...next[index]!, hex: newHex };
        return next;
      });
      const cssVar = DEFAULT_COLORS[index]!.cssVar;
      document.documentElement.style.setProperty(cssVar, newHex);
    },
    [],
  );

  const resetColors = useCallback(() => {
    setColors(DEFAULT_COLORS.map((c) => ({ ...c })));
    DEFAULT_COLORS.forEach((c) => {
      document.documentElement.style.setProperty(c.cssVar, c.hex);
    });
  }, []);

  return (
    <div className="flex h-full flex-col gap-section overflow-y-auto lg:overflow-hidden">
      <div className="shrink-0">
        <h1 className="text-page-title font-bold">{t("test.title")}</h1>
        <p className="mt-tight text-text-secondary">{t("test.subtitle")}</p>
      </div>

      <div className="grid gap-section md:grid-cols-2 lg:min-h-0 lg:flex-1">
        {/* Routing */}
        <Card>
          <div className="mb-element flex items-center gap-element">
            <StatusBadge />
            <h2 className="text-section-title font-semibold">{t("test.routingWorks")}</h2>
          </div>
          <p className="mb-element text-body text-text-secondary">{t("test.description")}</p>
          <div className="flex flex-wrap gap-element">
            {NAV_LINKS.map((link) => (
              <Link
                key={link.to}
                to={link.to}
                className="rounded-md bg-bg-tertiary px-tight py-tight text-body text-text-primary transition-colors hover:bg-accent hover:text-white"
              >
                {t(link.label)}
              </Link>
            ))}
          </div>
        </Card>

        {/* i18n */}
        <Card>
          <div className="mb-element flex items-center gap-element">
            <StatusBadge />
            <h2 className="text-section-title font-semibold">{t("test.i18nWorks")}</h2>
          </div>
          <p className="mb-element text-body text-text-secondary">
            {t("test.currentLocale")}:{" "}
            <span className="font-mono text-accent">{i18n.language}</span>
          </p>
          <div className="flex flex-wrap gap-element">
            {LOCALES.map((locale) => (
              <Button
                key={locale}
                variant={i18n.language === locale ? "default" : "secondary"}
                size="sm"
                onClick={() => i18n.changeLanguage(locale)}
              >
                {LOCALE_LABELS[locale as Locale]}
              </Button>
            ))}
          </div>
        </Card>

        {/* Theme / Color Picker */}
        <Card>
          <div className="mb-element flex items-center justify-between">
            <div className="flex items-center gap-element">
              <StatusBadge />
              <h2 className="text-section-title font-semibold">{t("test.themeWorks")}</h2>
            </div>
            <Button variant="ghost" size="sm" onClick={resetColors}>
              {t("test.reset")}
            </Button>
          </div>
          <div className="grid grid-cols-2 gap-x-section gap-y-tight sm:grid-cols-3 lg:grid-cols-4">
            {colors.map((swatch, i) => (
              <label
                key={swatch.name}
                className="group flex cursor-pointer items-center gap-element"
              >
                <div className="relative">
                  <div
                    className="size-swatch shrink-0 rounded-md border border-bg-tertiary transition-transform group-hover:scale-110"
                    style={{ backgroundColor: swatch.hex }}
                  />
                  <input
                    type="color"
                    value={swatch.hex}
                    onChange={(e) => handleColorChange(i, e.target.value)}
                    className="absolute inset-0 cursor-pointer opacity-0"
                  />
                </div>
                <div className="min-w-0">
                  <p className="truncate text-caption text-text-primary">{swatch.name}</p>
                  <p className="font-mono text-micro uppercase text-text-secondary">
                    {swatch.hex}
                  </p>
                </div>
              </label>
            ))}
          </div>
        </Card>

        {/* Components */}
        <Card>
          <div className="mb-element flex items-center gap-element">
            <StatusBadge />
            <h2 className="text-section-title font-semibold">{t("test.componentsWork")}</h2>
          </div>
          <p className="mb-element text-body text-text-secondary">
            {t("test.buttonVariants")}
          </p>
          <div className="flex flex-wrap gap-element">
            <Button variant="default">Default</Button>
            <Button variant="secondary">Secondary</Button>
            <Button variant="outline">Outline</Button>
            <Button variant="ghost">Ghost</Button>
            <Button variant="destructive">Destructive</Button>
            <Button variant="link">Link</Button>
          </div>
        </Card>
      </div>
    </div>
  );
}
