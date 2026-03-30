import type { LucideIcon } from "lucide-react";
import { cn } from "@/shared/utils/cn";

interface SettingItemProps {
  icon: LucideIcon;
  title: string;
  description?: string;
  control?: React.ReactNode;
  isLast?: boolean;
  className?: string;
}

export function SettingItem({
  icon: Icon,
  title,
  description,
  control,
  isLast = false,
  className,
}: SettingItemProps) {
  return (
    <div
      className={cn(
        "flex items-center gap-section px-section py-element",
        !isLast && "border-b border-bg-tertiary",
        className,
      )}
    >
      {/* Icon */}
      <div className="flex size-icon-md shrink-0 items-center justify-center rounded-md bg-accent/10">
        <Icon className="size-icon-sm text-accent" />
      </div>

      {/* Text */}
      <div className="min-w-0 flex-1">
        <p className="text-body font-medium text-text-primary">{title}</p>
        {description && (
          <p className="mt-hairline text-caption text-text-secondary">{description}</p>
        )}
      </div>

      {/* Control */}
      {control && <div className="shrink-0">{control}</div>}
    </div>
  );
}
