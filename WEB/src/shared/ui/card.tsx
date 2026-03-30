import { cn } from "@/shared/utils/cn";

interface CardProps extends React.HTMLAttributes<HTMLDivElement> {
  children: React.ReactNode;
}

export function Card({ className, children, ...props }: CardProps) {
  return (
    <div
      className={cn(
        "overflow-hidden rounded-lg border border-bg-tertiary bg-bg-secondary p-section",
        className,
      )}
      {...props}
    >
      {children}
    </div>
  );
}
