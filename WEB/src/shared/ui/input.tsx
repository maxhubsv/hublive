import * as React from "react";
import { cn } from "@/shared/utils/cn";

export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, ...props }, ref) => {
    return (
      <input
        ref={ref}
        className={cn(
          "w-full rounded-md border border-bg-tertiary bg-bg-primary px-tight py-element text-body text-text-primary placeholder:text-text-secondary focus:border-accent focus:outline-none",
          className,
        )}
        {...props}
      />
    );
  },
);
Input.displayName = "Input";

export { Input };
