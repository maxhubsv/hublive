import { NavLink, Outlet } from "react-router";
import { useTranslation } from "react-i18next";
import { cn } from "@/shared/utils/cn";
import { Button } from "@/shared/ui/button";
import { APP_NAME, LOCALES, LOCALE_LABELS, type Locale } from "@/shared/constants";
import { LayoutDashboard, Monitor, Radio, Settings } from "lucide-react";

const NAV_ITEMS: ReadonlyArray<{
  to: string;
  icon: typeof LayoutDashboard;
  labelKey: string;
  end?: boolean;
}> = [
  { to: "/", icon: LayoutDashboard, labelKey: "sidebar.dashboard", end: true },
  { to: "/streaming", icon: Radio, labelKey: "sidebar.streaming" },
  { to: "/settings", icon: Settings, labelKey: "sidebar.settings" },
];

export function MainLayout() {
  const { t, i18n } = useTranslation();

  return (
    <div className="flex h-screen overflow-hidden">
      {/* Sidebar */}
      <aside className="flex w-sidebar shrink-0 flex-col border-r border-bg-tertiary bg-bg-secondary">
        {/* Logo */}
        <div className="flex h-header items-center gap-element border-b border-bg-tertiary px-page">
          <div className="flex size-icon-md items-center justify-center rounded-lg bg-accent">
            <Monitor className="size-icon-sm text-white" />
          </div>
          <span className="text-section-title font-semibold">{APP_NAME}</span>
        </div>

        {/* Navigation */}
        <nav className="flex-1 space-y-hairline p-tight">
          {NAV_ITEMS.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.end}
              className={({ isActive }) =>
                cn(
                  "flex items-center gap-tight rounded-md px-tight py-element text-body font-medium transition-colors",
                  isActive
                    ? "bg-accent/10 text-accent"
                    : "text-text-secondary hover:bg-bg-tertiary hover:text-text-primary",
                )
              }
            >
              <item.icon className="size-icon-sm" />
              {t(item.labelKey)}
            </NavLink>
          ))}
        </nav>
      </aside>

      {/* Main content area */}
      <div className="flex flex-1 flex-col overflow-hidden">
        {/* Header */}
        <header className="flex h-header shrink-0 items-center justify-end border-b border-bg-tertiary bg-bg-secondary px-page">
          <div className="flex items-center gap-hairline">
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
        </header>

        {/* Page content */}
        <main className="min-h-0 flex-1 overflow-hidden p-page">
          <Outlet />
        </main>
      </div>
    </div>
  );
}
