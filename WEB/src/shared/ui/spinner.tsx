import { cn } from "@/shared/utils/cn";

interface SpinnerProps {
  className?: string;
}

export function Spinner({ className }: SpinnerProps) {
  return (
    <div
      className={cn(
        "size-spinner animate-spin rounded-full border-2 border-accent border-t-transparent",
        className,
      )}
    />
  );
}
