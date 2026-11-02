export type PomodoroPhase = "work" | "short_break" | "long_break"

/** 与后端 timer::PomodoroState 对齐，也是 pomodoro-tick 事件 payload。 */
export interface PomodoroState {
  phase: PomodoroPhase
  /** 当前阶段剩余秒数。 */
  remaining_seconds: number
  /** 当前阶段总秒数，用于前端计算进度环。 */
  total_seconds: number
  is_running: boolean
  /** 已完成的 work 阶段数量；达到阈值后进入 long_break。 */
  completed_sessions: number
  /** true = 上次退出时计时器正在运行，需用户处理中断。 */
  interrupted: boolean
  /** 中断对应的活跃 session id（如果有）。 */
  interrupted_session_id: number | null
  /** 上次记录状态的 UTC ISO 8601 时间戳，用于 UI 显示 "上次运行于..."。 */
  last_seen_at: string | null
}

/** 与 commands::pomodoro::PomodoroConfigData 对齐，不包含数据库内部 id。 */
export interface PomodoroConfig {
  /** 工作阶段时长，单位秒。 */
  work_seconds: number
  /** 短休息时长，单位秒。 */
  short_break_seconds: number
  /** 长休息时长，单位秒。 */
  long_break_seconds: number
  /** 每完成多少个 work 阶段触发一次 long_break。 */
  sessions_before_long_break: number
  /** 阶段结束后是否自动开始下一阶段计时；false = 暂停等待用户按开始（默认）。 */
  auto_start_next_phase: boolean
}

/** resolve_pomodoro_interruption 的入参。 */
export interface ResolvePomodoroInterruptionRequest {
  action: "continue" | "discard" | "complete"
}
