import { Separator as SeparatorPrimitive } from "radix-ui";
import { cn } from "@/shared/utils/cn";

interface SeparatorProps
  extends React.ComponentProps<typeof SeparatorPrimitive.Root> {}

export function Separator({
  className,
  orientation = "horizontal",
  decorative = true,
  ...props
}: SeparatorProps) {
  return (
    <SeparatorPrimitive.Root
      decorative={decorative}
      orientation={orientation}
      className={cn(
        "shrink-0 bg-bg-tertiary",
        orientation === "horizontal" ? "h-px w-full" : "w-px self-stretch",
        className,
      )}
      {...props}
    />
  );
}
