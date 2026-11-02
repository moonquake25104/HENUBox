import { Capacitor, registerPlugin } from "@capacitor/core"

export type FocusMode = "focus" | "countup" | "short_break" | "long_break"
export type FocusSettings = {
  focus_minutes: number; short_minutes: number; long_minutes: number; long_every: number
  auto_break: boolean; auto_focus: boolean; sound: boolean; vibrate: boolean
}
export type FocusRecord = { id: string; mode: FocusMode; label: string; started_at: number; ended_at: number; actual_ms: number; completed: boolean }
export type FocusTimer = { id: string; mode: FocusMode; label: string; status: "RUNNING" | "PAUSED" | "INTERRUPTED"; started_at: number; elapsed_ms: number; remaining_ms: number; planned_ms: number }
export type FocusState = {
  schema_version: 1; revision: number; settings: FocusSettings; timer: FocusTimer | null
  labels: string[]; round: number; next_mode: FocusMode; interruption?: boolean; notification_permission: boolean
  today: { seconds: number; pomodoros: number }; week: { date: string; seconds: number; pomodoros: number }[]
  records: FocusRecord[]; last_finished?: FocusRecord; open_requested?: boolean
  heatmap?: { from: string; to: string; today: string; total_seconds: number; days: { date: string; seconds: number; pomodoros: number }[] }
}
export const FOCUS_NAMES: Record<FocusMode, string> = { focus: "番茄专注", countup: "自由专注", short_break: "短休息", long_break: "长休息" }
interface FocusBridge {
  getState(): Promise<FocusState>
  command(value: { action: "start" | "pause" | "resume" | "finish" | "tag" | "archive"; expected_revision: number; request_id: string; mode: string; label: string }): Promise<FocusState>
  saveSettings(value: { expected_revision: number; settings: FocusSettings }): Promise<FocusState>
}
export const focusBridge = registerPlugin<FocusBridge>("XiqueFocus")
export const focusAvailable = () => Capacitor.isPluginAvailable("XiqueFocus")
export function focusTime(ms: number) {
  const seconds = Math.max(0, Math.ceil(ms / 1000))
  const minutes = Math.floor(seconds / 60)
  return `${minutes < 10 ? "0" : ""}${minutes}:${String(seconds % 60).padStart(2, "0")}`
}
export function focusDuration(seconds: number) {
  if (seconds < 60) return `${Math.floor(seconds)} 秒`
  const m = Math.floor(seconds / 60)
  return m >= 60 ? `${Math.floor(m / 60)} 小时 ${m % 60} 分` : `${m} 分钟`
}
export function focusError(e: unknown) {
  const code = (e as { code?: string })?.code
  const messages: Record<string, string> = {
    REVISION_CONFLICT: "计时状态已变化，已重新读取，请再操作一次。",
    TIMER_ACTIVE: "请先结束当前计时，再修改设置或开启新的计时。",
    INVALID_ARGUMENT: "请检查时长和标签：专注 1–180 分钟，休息 1–60 分钟，标签最多 40 字。",
    LABEL_LIMIT: "标签最多 64 个，可以归档不再使用的标签。",
    BACKGROUND_UNAVAILABLE: "后台计时未能启动，已暂停。请重新打开 App 后继续。",
    INVALID_STATE: "计时状态已变化，请刷新后重试。",
  }
  return messages[code ?? ""] ?? "无法读取或保存专注数据，请重试。"
}
