import type { ComponentPropsWithoutRef } from "react"
import { cn } from "@/lib/utils"

export function AcrylicPanel({ children, className, ...props }: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      className={cn(
        "rounded-xl border border-border/60",
        "bg-card/80 backdrop-blur-xl backdrop-saturate-150",
        "glass-edge",
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}
