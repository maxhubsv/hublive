import { Outlet } from "react-router";
import { ErrorBoundary } from "@/shared/components/ErrorBoundary";

export function RootLayout() {
  return (
    <ErrorBoundary>
      <Outlet />
    </ErrorBoundary>
  );
}
