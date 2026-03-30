import { lazy, Suspense } from "react";
import { createBrowserRouter } from "react-router";
import { RootLayout } from "@/layouts/RootLayout";
import { MainLayout } from "@/layouts/MainLayout";
import { AuthLayout } from "@/layouts/AuthLayout";

// Lazy-loaded pages for code splitting
const TestPage = lazy(() => import("@/features/dashboard/pages/TestPage"));
const DashboardPage = lazy(
  () => import("@/features/dashboard/pages/DashboardPage"),
);
const StreamingPage = lazy(
  () => import("@/features/streaming/pages/StreamingPage"),
);
const LoginPage = lazy(() => import("@/features/auth/pages/LoginPage"));

function PageLoader() {
  return (
    <div className="flex h-full items-center justify-center">
      <div className="size-spinner animate-spin rounded-full border-2 border-accent border-t-transparent" />
    </div>
  );
}

function SuspenseWrap({ children }: { children: React.ReactNode }) {
  return <Suspense fallback={<PageLoader />}>{children}</Suspense>;
}

export const router = createBrowserRouter([
  {
    element: <RootLayout />,
    children: [
      {
        path: "/login",
        element: <AuthLayout />,
        children: [
          {
            index: true,
            element: (
              <SuspenseWrap>
                <LoginPage />
              </SuspenseWrap>
            ),
          },
        ],
      },
      {
        element: <MainLayout />,
        children: [
          {
            path: "/",
            element: (
              <SuspenseWrap>
                <TestPage />
              </SuspenseWrap>
            ),
          },
          {
            path: "/dashboard",
            element: (
              <SuspenseWrap>
                <DashboardPage />
              </SuspenseWrap>
            ),
          },
          {
            path: "/streaming",
            element: (
              <SuspenseWrap>
                <StreamingPage />
              </SuspenseWrap>
            ),
          },
        ],
      },
    ],
  },
]);
