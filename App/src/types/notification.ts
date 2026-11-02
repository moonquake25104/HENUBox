/** 与后端 db::models::NotificationConfig 对齐，是本地通知的全局配置记录。 */
export interface NotificationConfig {
  id: number
  /** 全局通知开关，true = 启用。 */
  enabled: boolean
  /** 考试提醒偏移量（JSON 数组，单位分钟），例如 [1440,60] 表示提前 1 天和提前 1 小时。 */
  exam_offsets_json: string
  /** 课程提醒偏移量（JSON 数组，单位分钟），例如 [10] 表示提前 10 分钟。 */
  course_offsets_json: string
  /** Android 通知渠道是否已创建（幂等保护）。 */
  android_channel_created: boolean
  /** UTC ISO 8601 创建时间。 */
  created_at: string
  /** UTC ISO 8601 更新时间。 */
  updated_at: string
}

/** 与后端 db::models::UpdateNotificationConfig 对齐，所有字段可选以支持局部更新。 */
export interface UpdateNotificationConfig {
  enabled?: boolean
  exam_offsets_json?: string
  course_offsets_json?: string
  android_channel_created?: boolean
}

export type NotificationPermissionState =
  | "granted"
  | "denied"
  | "prompt"
  | "prompt-with-rationale"

export type TermPhaseType = "teaching" | "exam" | "break"

/** 与后端 db::models::TermPhase 对齐。 */
export interface TermPhase {
  id: number
  /** 跨设备稳定标识（UUID）。 */
  sync_id: string
  /** 学期标识，例如 2026S1。 */
  term_label: string
  phase_type: TermPhaseType
  /** 起始教学周，最小为 1。 */
  start_week: number
  /** 结束教学周，必须大于等于 start_week。 */
  end_week: number
  /** true = 课程在该阶段可见。 */
  affects_courses: boolean
  /** true = 考试通知在该阶段可用。 */
  affects_exam_notifications: boolean
  /** 关联 pomodoro profile 名称。 */
  pomodoro_profile: string
  /** 通知规则 JSON，当前默认 {}。 */
  notification_rules_json: string
  sort_order: number
  /** 墓碑时间戳；null = 活跃。 */
  deleted_at: string | null
  /** UTC ISO 8601 创建时间。 */
  created_at: string
  /** UTC ISO 8601 更新时间。 */
  updated_at: string
}

export interface CreateTermPhaseRequest {
  term_label: string
  phase_type: TermPhaseType
  start_week: number
  end_week: number
  affects_courses: boolean
  affects_exam_notifications: boolean
  pomodoro_profile: string
  notification_rules_json: string
  sort_order: number
}

export type UpdateTermPhaseRequest = CreateTermPhaseRequest

/** 与后端 db::models::PomodoroProfile 对齐。 */
export interface PomodoroProfile {
  id: number
  name: string
  work_seconds: number
  short_break_seconds: number
  long_break_seconds: number
  sessions_before_long_break: number
  /** true = 内置 profile，不能删除。 */
  is_builtin: boolean
  /** UTC ISO 8601 创建时间。 */
  created_at: string
  /** UTC ISO 8601 更新时间。 */
  updated_at: string
}

export interface CreatePomodoroProfileRequest {
  name: string
  work_seconds: number
  short_break_seconds: number
  long_break_seconds: number
  sessions_before_long_break: number
}

export type UpdatePomodoroProfileRequest = CreatePomodoroProfileRequest

/** 与后端 term_phase::CurrentPhaseStatus 对齐。 */
export interface CurrentPhaseStatus {
  source: string
  term_label: string | null
  phase_type: "unknown" | TermPhaseType
  current_week: number | null
  start_week: number | null
  end_week: number | null
  courses_visible: boolean
  exam_notifications_enabled: boolean
  pomodoro_profile: string
  inferred: boolean
}

/** 与后端 db::models::SemesterContext 对齐，用于阶段设置页选择学期。 */
export interface SemesterContext {
  id: number
  source: string
  academic_year: string | null
  term: string | null
  term_label: string
  /** 学期锚点日期，格式 YYYY-MM-DD。 */
  start_date: string
  /** 当前周；null 表示未知。 */
  current_week: number | null
  /** 总周次；null 表示未知。 */
  total_weeks: number | null
  /** UTC ISO 8601 最后刷新时间。 */
  refreshed_at: string
  /** UTC ISO 8601 创建时间。 */
  created_at: string
  /** UTC ISO 8601 更新时间。 */
  updated_at: string
}
