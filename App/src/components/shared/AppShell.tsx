import type { ReactNode } from "react"
import type { LucideIcon } from "lucide-react"
import { CalendarDays, CalendarIcon, CheckSquare, Cloud, Moon, Settings, Sun, Timer } from "lucide-react"
import { cn } from "@/lib/utils"
import { useTheme } from "@/hooks/use-theme"
import kairosLogo from "@/assets/kairos-logo.svg"

interface NavItem {
  key: string
  label: string
  icon?: LucideIcon
  logoSrc?: string
}

/** 桌面侧边栏导航，与移动端主入口保持一致 */
const DESKTOP_NAV: NavItem[] = [
  { key: "today", label: "今天", icon: CalendarDays },
  { key: "pomodoro", label: "专注", icon: Timer },
  { key: "todo", label: "待办事项", icon: CheckSquare },
  { key: "calendar", label: "日历", icon: CalendarIcon },
  { key: "kairos", label: "设置", icon: Settings },
]

/** 移动端底部主入口（5 个） */
const MOBILE_MAIN: NavItem[] = [
  { key: "today", label: "今天", icon: CalendarDays },
  { key: "pomodoro", label: "专注", icon: Timer },
  { key: "todo", label: "待办", icon: CheckSquare },
  { key: "calendar", label: "日历", icon: CalendarIcon },
  { key: "kairos", label: "设置", icon: Settings },
]

interface AppShellProps {
  active: string
  onNavigate: (key: string) => void
  children: ReactNode
}

function NavButton({
  active,
  icon: Icon,
  logoSrc,
  label,
  onClick,
}: {
  active: boolean
  icon?: LucideIcon
  logoSrc?: string
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? "page" : undefined}
      className={cn(
        "flex h-12 min-w-16 flex-col items-center justify-center gap-0.5 rounded-xl px-2 py-1.5 text-[10px] font-medium transition-colors",
        "md:flex-row md:gap-2.5 md:px-2.5 md:py-2 md:text-sm md:rounded-lg",
        active
          ? "text-primary bg-primary/10 md:bg-primary/[0.12]"
          : "text-foreground/70 hover:text-foreground md:hover:bg-muted/60",
      )}
    >
      {logoSrc ? (
        <img
          src={logoSrc}
          alt=""
          aria-hidden="true"
          className={cn("h-5 w-5 shrink-0 rounded transition-transform md:h-4 md:w-4", active && "scale-110")}
        />
      ) : Icon ? (
        <Icon className={cn("h-5 w-5 shrink-0 transition-transform md:h-4 md:w-4", active && "scale-110")} />
      ) : null}
      {label}
    </button>
  )
}

/** 桌面侧边栏导航按钮 */
function SidebarNavButton({
  active,
  icon: Icon,
  logoSrc,
  label,
  onClick,
}: {
  active: boolean
  icon?: LucideIcon
  logoSrc?: string
  label: string
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? "page" : undefined}
      className={cn(
        "group flex items-center gap-2.5 rounded-lg px-2.5 py-2 text-sm font-medium transition-all",
        active
          ? "bg-primary/[0.12] text-primary"
          : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
      )}
    >
      {logoSrc ? (
        <img
          src={logoSrc}
          alt=""
          aria-hidden="true"
          className="h-4 w-4 shrink-0 rounded transition-transform group-hover:scale-110"
        />
      ) : Icon ? (
        <Icon className="h-4 w-4 shrink-0 transition-transform group-hover:scale-110" />
      ) : null}
      {label}
    </button>
  )
}

/** 应用外壳：桌面侧边栏 + 移动端底部 Tab Bar。Android 安全区域由 MainActivity insets 处理。 */
export function AppShell({ active, onNavigate, children }: AppShellProps) {
  const { theme, toggle } = useTheme()
  const isActive = (key: string) => active === key
  const isKairosArea = [
    "kairos",
    "courses",
    "exams",
    "notifications",
    "semester-phases",
    "ai-settings",
    "sync",
  ].includes(active)

  return (
    <div className="relative z-0 flex flex-col h-screen overflow-hidden">
      {/* 主体：桌面侧边栏 + 内容区 */}
      <div className="flex min-h-0 flex-1 overflow-hidden">
        {/* 桌面侧边栏：≥md 时显示 */}
        <aside className="hidden md:flex w-52 shrink-0 flex-col border-r border-border/50 bg-card/40 px-3 py-4 backdrop-blur-xl">
          <div className="flex items-center gap-2 px-2 pb-3.5">
            <img src={kairosLogo} alt="HENUBox" className="h-8 w-8 shrink-0 rounded-lg" />
            <div className="min-w-0">
              <div className="font-heading text-sm font-medium leading-none text-primary">HENUBox</div>
              <div className="mt-1 line-clamp-2 text-[10px] text-muted-foreground">
                好好上课，也好好生活。
              </div>
            </div>
          </div>

          <nav className="flex flex-1 flex-col gap-1">
            {DESKTOP_NAV.map(({ key, label, icon: Icon, logoSrc }) => (
              <SidebarNavButton
                key={key}
                active={isActive(key)}
                icon={Icon}
                logoSrc={logoSrc}
                label={label}
                onClick={() => onNavigate(key)}
              />
            ))}
          </nav>

          <div className="mt-2 flex items-center justify-between border-t border-border/50 pt-3">
            <button
              type="button"
              onClick={() => onNavigate("sync")}
              aria-current={isActive("sync") ? "page" : undefined}
              className={cn(
                "flex items-center gap-2 rounded-lg px-2.5 py-2 text-sm font-medium transition-colors",
                isActive("sync")
                  ? "bg-primary/[0.12] text-primary"
                  : "text-muted-foreground hover:bg-muted/60 hover:text-foreground",
              )}
            >
              <Cloud className="h-4 w-4" />
              同步
            </button>
            <button
              type="button"
              onClick={toggle}
              aria-label="切换深浅色模式"
              className="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
            >
              {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
          </div>
        </aside>

        {/* 内容区：底部 Tab Bar 是普通 flex 子项，不需要额外用 padding 抵消。 */}
        <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
          <div className="flex min-h-0 flex-1 flex-col overflow-hidden px-4 pb-0 pt-6 md:px-6 md:py-8">
            {children}
          </div>
        </main>
      </div>

      {/* 移动端底部 Tab Bar：仅在 <md 时显示 */}
      <nav className="flex md:hidden shrink-0 items-center justify-around h-16 border-t border-border/50 bg-card/40 backdrop-blur-xl relative">
        {MOBILE_MAIN.map(({ key, label, icon: Icon, logoSrc }) => {
          const isItemActive = key === "kairos" ? isKairosArea : isActive(key)
          return (
            <NavButton
              key={key}
              active={isItemActive}
              icon={Icon}
              logoSrc={logoSrc}
              label={label}
              onClick={() => onNavigate(key)}
            />
          )
        })}
      </nav>
    </div>
  )
}
