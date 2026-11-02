import { useState } from "react"
import {
  Bell,
  BookOpen,
  CalendarClock,
  Check,
  Cloud,
  Flag,
  LogOut,
  Moon,
  Palette,
  Settings,
  Sparkles,
  Sun,
} from "lucide-react"
import { AcrylicPanel } from "@/components/shared/acrylic-panel"
import { Button } from "@/components/ui/button"
import { ACCENT_OPTIONS, useTheme } from "@/hooks/use-theme"
import { cn } from "@/lib/utils"
import { logout } from "@/lib/api/auth"
import { clearToken } from "@/lib/api/client"
import kairosLogo from "@/assets/kairos-logo.svg"

interface KairosHubProps {
  onNavigate: (key: string) => void
  className?: string
}

interface HubEntry {
  key: string
  label: string
  description: string
  icon: typeof BookOpen
}

const HUB_ENTRIES: HubEntry[] = [
  {
    key: "courses",
    label: "课程表",
    description: "管理课程、周次与上课地点",
    icon: BookOpen,
  },
  {
    key: "exams",
    label: "考试倒计时",
    description: "查看考试时间、地点与剩余天数",
    icon: CalendarClock,
  },
  {
    key: "notifications",
    label: "通知设置",
    description: "管理番茄钟提醒与考试考前通知",
    icon: Bell,
  },
  {
    key: "semester-phases",
    label: "学期阶段",
    description: "管理教学周、考试周与假期行为",
    icon: Flag,
  },
  {
    key: "ai-settings",
    label: "AI 设置",
    description: "配置每日摘要与 API Key",
    icon: Sparkles,
  },
  {
    key: "sync",
    label: "同步设置",
    description: "配置 WebDAV 与本地数据同步",
    icon: Cloud,
  },
]

export function KairosHub({ onNavigate, className }: KairosHubProps) {
  const { theme, accent, toggle, setAccent } = useTheme()
  const ThemeIcon = theme === "dark" ? Sun : Moon
  const activeAccent = ACCENT_OPTIONS.find((option) => option.value === accent)
  const [loggingOut, setLoggingOut] = useState(false)

  const handleLogout = async () => {
    if (loggingOut) return
    setLoggingOut(true)
    await logout().catch(() => {})
    clearToken()
    window.dispatchEvent(new Event("kairos:unauthorized"))
  }

  return (
    <div className={cn("min-h-0 flex-1 overflow-y-auto pb-4", className)}>
      <div className="mx-auto flex w-full max-w-2xl flex-col gap-4">
        <AcrylicPanel className="bg-card p-5">
          <div className="flex items-center gap-4">
            <img src={kairosLogo} alt="HENUBox" className="h-14 w-14 shrink-0 rounded-xl" />
            <div className="min-w-0 flex-1">
              <h1 className="font-heading text-xl font-semibold leading-tight text-foreground">
                HENUBox
              </h1>
              <p className="mt-1 text-sm text-muted-foreground">
                好好上课，也好好生活。
              </p>
            </div>
          </div>
        </AcrylicPanel>

        <AcrylicPanel className="flex flex-col gap-4 bg-card p-4">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 items-center gap-3">
              <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
                <Settings className="h-5 w-5" />
              </span>
              <div className="min-w-0">
                <div className="text-sm font-semibold text-foreground">外观</div>
                <div className="text-xs text-muted-foreground">
                  当前为{theme === "dark" ? "深色" : "浅色"}模式
                </div>
              </div>
            </div>
            <Button type="button" variant="outline" size="sm" onClick={toggle}>
              <ThemeIcon className="mr-1.5 h-3.5 w-3.5" />
              切换
            </Button>
          </div>

          <div className="flex flex-col gap-2 border-t border-border/50 pt-3">
            <div className="flex items-center justify-between gap-3">
              <div className="flex min-w-0 items-center gap-2 text-sm font-semibold text-foreground">
                <Palette className="h-4 w-4 text-muted-foreground" />
                主题色
              </div>
              <span className="text-xs text-muted-foreground">{activeAccent?.label ?? "清蓝"}</span>
            </div>
            <div className="grid grid-cols-6 gap-2">
              {ACCENT_OPTIONS.map((option) => {
                const isActive = option.value === accent

                return (
                  <button
                    key={option.value}
                    type="button"
                    onClick={() => setAccent(option.value)}
                    aria-label={`切换为${option.label}主题色`}
                    aria-pressed={isActive}
                    className={cn(
                      "flex min-h-11 min-w-11 items-center justify-center rounded-lg border border-border/70 bg-muted/40 transition-colors hover:bg-muted",
                      isActive && "border-primary bg-primary/10",
                    )}
                  >
                    <span className="flex h-6 w-6 items-center justify-center rounded-full text-white shadow-sm"
                      style={{ backgroundColor: option.swatchColor }}
                    >
                      {isActive && <Check className="h-3.5 w-3.5" />}
                    </span>
                  </button>
                )
              })}
            </div>
          </div>
        </AcrylicPanel>

        <div className="grid gap-3">
          {HUB_ENTRIES.map(({ key, label, description, icon: Icon }) => (
            <button
              key={key}
              type="button"
              onClick={() => onNavigate(key)}
              className={cn(
                "flex min-h-16 w-full items-center gap-3 rounded-lg border border-border/60 bg-card/70 px-4 py-3 text-left",
                "transition-colors hover:bg-card active:bg-muted/60",
              )}
            >
              <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Icon className="h-5 w-5" />
              </span>
              <span className="min-w-0 flex-1">
                <span className="block text-sm font-semibold text-foreground">{label}</span>
                <span className="mt-0.5 block truncate text-xs text-muted-foreground">
                  {description}
                </span>
              </span>
            </button>
          ))}
        </div>

        <button
          type="button"
          onClick={() => void handleLogout()}
          disabled={loggingOut}
          className={cn(
            "flex min-h-16 w-full items-center gap-3 rounded-lg border border-border/60 bg-card/70 px-4 py-3 text-left",
            "transition-colors hover:bg-card active:bg-muted/60 disabled:pointer-events-none disabled:opacity-60",
          )}
        >
          <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
            <LogOut className="h-5 w-5" />
          </span>
          <span className="min-w-0 flex-1">
            <span className="block text-sm font-semibold text-foreground">
              {loggingOut ? "正在退出…" : "退出登录"}
            </span>
            <span className="mt-0.5 block truncate text-xs text-muted-foreground">
              清除本机登录状态并返回登录页
            </span>
          </span>
        </button>

        <p className="text-center text-[11px] text-muted-foreground/50">
          v{__APP_VERSION__}
        </p>
      </div>
    </div>
  )
}
