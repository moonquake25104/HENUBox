import type { ReactNode } from "react"
import { cn } from "@/lib/utils"

interface PageShellProps {
  /** 页面标题，显示在顶部居中 */
  title: string
  /** 子内容 */
  children: ReactNode
  /** 内容最大宽度：sm (384px), md (448px), lg (512px), 2xl (672px) */
  width?: "sm" | "md" | "lg" | "2xl"
  /** 是否垂直居中（默认顶对齐） */
  centered?: boolean
  /** 标题栏右侧操作按钮，不传则留空 */
  action?: ReactNode
  /** 标题额外样式类，可用于增大字号等；默认 text-lg font-heading font-medium */
  titleClassName?: string
}

const WIDTH_CLASS: Record<NonNullable<PageShellProps["width"]>, string> = {
  sm: "max-w-sm",
  md: "max-w-md",
  lg: "max-w-lg",
  "2xl": "max-w-2xl",
}

/** 页面外壳：统一标题栏 + 入场动画 + 宽度容器 + 间距。专注/待办/课程/考试/同步均可复用。 */
export function PageShell({ title, children, width = "2xl", centered = false, action, titleClassName }: PageShellProps) {
  return (
    <div
      className={
        `mx-auto flex w-full ${WIDTH_CLASS[width]} flex-col gap-4 animate-in fade-in-0 slide-in-from-bottom-2 duration-300`
      }
    >
      {/* 标题栏 */}
      <div className="flex items-center justify-between gap-2 min-h-[2rem]">
        <h2 className={cn("text-lg font-heading font-medium text-foreground", titleClassName)}>{title}</h2>
        {action && <div className="flex items-center">{action}</div>}
      </div>

      {/* 内容 */}
      <div className={centered ? "flex flex-1 items-center justify-center" : undefined}>
        {children}
      </div>
    </div>
  )
}
