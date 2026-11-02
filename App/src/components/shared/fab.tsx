import { Plus } from "lucide-react"
import { cn } from "@/lib/utils"

interface FabProps {
  onClick: () => void
  "aria-label"?: string
  className?: string
}

/** 页面右下角悬浮按钮 (FAB)，基于 primary 主题色，带阴影和 hover/active 缩放动效。 */
export function Fab({ onClick, "aria-label": ariaLabel = "新建任务", className }: FabProps) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-label={ariaLabel}
      className={cn(
        "fixed right-4 bottom-20 z-40",
        "h-14 w-14 rounded-full",
        "bg-primary text-primary-foreground",
        "shadow-lg shadow-primary/25",
        "flex items-center justify-center",
        "hover:scale-105 active:scale-95 transition-transform",
        "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring",
        className,
      )}
    >
      <Plus className="h-6 w-6" />
    </button>
  )
}
