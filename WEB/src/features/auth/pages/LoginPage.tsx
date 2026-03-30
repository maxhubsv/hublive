import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@/shared/ui/button";
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
            className="mb-[4px] block text-body font-medium text-text-secondary"
          >
            {t("auth.email")}
          </label>
          <input
            id="email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            className="w-full rounded-md border border-bg-tertiary bg-bg-primary px-tight py-element text-body text-text-primary placeholder:text-text-secondary focus:border-accent focus:outline-none"
            placeholder="admin@hublive.io"
          />
        </div>
        <div>
          <label
            htmlFor="password"
            className="mb-[4px] block text-body font-medium text-text-secondary"
          >
            {t("auth.password")}
          </label>
          <input
            id="password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            className="w-full rounded-md border border-bg-tertiary bg-bg-primary px-tight py-element text-body text-text-primary placeholder:text-text-secondary focus:border-accent focus:outline-none"
          />
        </div>
        <Button type="submit" className="w-full" disabled={login.isPending}>
          {login.isPending ? t("app.loading") : t("auth.loginButton")}
        </Button>
      </form>
    </div>
  );
}
