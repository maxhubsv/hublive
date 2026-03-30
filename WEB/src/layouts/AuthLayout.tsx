import { Outlet } from "react-router";
import { Monitor } from "lucide-react";
import { APP_NAME } from "@/shared/constants";

export function AuthLayout() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-bg-primary p-section">
      <div className="w-full max-w-md rounded-lg border border-bg-tertiary bg-bg-secondary p-section sm:p-page">
        <div className="mb-page flex items-center justify-center gap-element">
          <div className="flex size-icon-lg items-center justify-center rounded-lg bg-accent">
            <Monitor className="size-icon-md text-white" />
          </div>
          <span className="text-page-title font-semibold">{APP_NAME}</span>
        </div>
        <Outlet />
      </div>
    </div>
  );
}
