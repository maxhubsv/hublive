import { Outlet } from "react-router";
import { APP_NAME } from "@/shared/constants";

export function AuthLayout() {
  return (
    <div className="flex min-h-screen items-center justify-center bg-bg-primary">
      <div className="w-full max-w-md rounded-lg border border-bg-tertiary bg-bg-secondary p-page">
        <div className="mb-page flex items-center justify-center gap-element">
          <div className="flex size-icon-lg items-center justify-center rounded-lg bg-accent">
            <span className="text-section-title font-bold text-white">H</span>
          </div>
          <span className="text-page-title font-semibold">{APP_NAME}</span>
        </div>
        <Outlet />
      </div>
    </div>
  );
}
