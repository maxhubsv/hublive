import { Component, type ErrorInfo, type ReactNode } from "react";
import { AlertCircle } from "lucide-react";
import i18n from "@/lib/i18n";
import { Button } from "@/shared/ui/button";

interface ErrorBoundaryProps {
  children: ReactNode;
  fallback?: ReactNode;
}

interface ErrorBoundaryState {
  hasError: boolean;
  error?: Error;
}

export class ErrorBoundary extends Component<ErrorBoundaryProps, ErrorBoundaryState> {
  constructor(props: ErrorBoundaryProps) {
    super(props);
    this.state = { hasError: false };
  }

  static getDerivedStateFromError(error: Error): ErrorBoundaryState {
    return { hasError: true, error };
  }

  componentDidCatch(error: Error, errorInfo: ErrorInfo) {
    console.error("[ErrorBoundary]", error, errorInfo);
  }

  render() {
    if (this.state.hasError) {
      if (this.props.fallback) {
        return this.props.fallback;
      }

      return (
        <div className="flex h-full items-center justify-center p-page">
          <div className="w-full max-w-md rounded-lg bg-bg-secondary p-page text-center">
            <div className="mb-section flex justify-center">
              <AlertCircle className="size-icon-lg text-danger" />
            </div>
            <h2 className="mb-element text-section-title font-semibold text-danger">
              {i18n.t("app.somethingWentWrong")}
            </h2>
            <p className="mb-section text-body text-text-secondary">
              {this.state.error?.message || i18n.t("app.unexpectedError")}
            </p>
            <Button
              onClick={() => this.setState({ hasError: false, error: undefined })}
            >
              {i18n.t("app.tryAgain")}
            </Button>
          </div>
        </div>
      );
    }

    return this.props.children;
  }
}
