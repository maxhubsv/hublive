import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { useLogin } from "../hooks/useAuth";

export default function LoginPage() {
  const { t } = useTranslation();
  const login = useLogin();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    login.mutate({ email, password });
  };

  return (
    <div>
      <h1 className="mb-page text-center text-page-title font-bold text-text-primary">
        {t("auth.login")}
      </h1>
      <form onSubmit={handleSubmit} className="space-y-section">
        <div>
          <label
            htmlFor="email"
            className="mb-micro block text-body font-medium text-text-secondary"
          >
            {t("auth.email")}
          </label>
          <Input
            id="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder={t("auth.emailPlaceholder")}
          />
        </div>
        <div>
          <label
            htmlFor="password"
            className="mb-micro block text-body font-medium text-text-secondary"
          >
            {t("auth.password")}
          </label>
          <Input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>
        <Button type="submit" className="w-full" disabled={login.isPending}>
          {login.isPending ? t("app.loading") : t("auth.loginButton")}
        </Button>
      </form>
    </div>
  );
}
