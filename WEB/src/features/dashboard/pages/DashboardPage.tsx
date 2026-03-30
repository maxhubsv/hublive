import { useTranslation } from "react-i18next";

export default function DashboardPage() {
  const { t } = useTranslation();

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <h1 className="text-page-title font-bold">{t("dashboard.title")}</h1>
      <p className="mt-element text-text-secondary">{t("dashboard.welcome")}</p>
    </div>
  );
}
