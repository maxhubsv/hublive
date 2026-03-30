import { Check, AlertTriangle, AlertCircle, Info } from "lucide-react";
import { cn } from "@/shared/utils/cn";

interface StatusBadgeProps {
  variant?: "success" | "danger" | "warning" | "accent";
  icon?: React.ReactNode;
  className?: string;
}

const DEFAULT_ICONS = {
  success: <Check className="size-icon-sm" />,
  danger: <AlertCircle className="size-icon-sm" />,
  warning: <AlertTriangle className="size-icon-sm" />,
  accent: <Info className="size-icon-sm" />,
};

export function StatusBadge({
  variant = "success",
  icon,
  className,
}: StatusBadgeProps) {
  const variantStyles = {
    success: "bg-success/20 text-success",
    danger: "bg-danger/20 text-danger",
    warning: "bg-warning/20 text-warning",
    accent: "bg-accent/20 text-accent",
  };

  return (
    <span
      className={cn(
        "flex size-icon-md items-center justify-center rounded-full",
        variantStyles[variant],
        className,
      )}
    >
      {icon ?? DEFAULT_ICONS[variant]}
    </span>
  );
}
