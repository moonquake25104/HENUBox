/** 与后端 schedule::WeekScheduleItem / WeekScheduleResponse 对齐（src-tauri/src/schedule.rs）。 */
export interface WeekScheduleItem {
  /** 聚合来源：course 来自课程表，exam 来自考试表。 */
  kind: "course" | "exam"
  id: number
  title: string
  /** ISO weekday: 1 = 周一，7 = 周日。 */
  day_of_week: number
  /** 当天开始时间，格式 HH:mm。 */
  start_time: string
  /** 当天结束时间，格式 HH:mm。 */
  end_time: string
  location: string
  teacher: string
  /** 展示颜色，优先使用课程颜色；无课程关联的考试使用后端 fallback 色。 */
  color: string
  notes: string
  /** 课程周次规则；考试项为空字符串。 */
  week_pattern: string
  /** 关联课程本地 SQLite id；课程项为自身 id，未绑定课程的考试为 null。 */
  course_id: number | null
}

export interface WeekScheduleResponse {
  /** 请求的教学周序号，最小为 1。 */
  week_index: number
  /** 学期标识，例如 2026S1。 */
  semester: string
  /** 学期锚点日期，格式 YYYY-MM-DD。 */
  semester_start_date: string
  /** 当前周周一日期，格式 YYYY-MM-DD。 */
  week_start_date: string
  /** 当前周周日日期，格式 YYYY-MM-DD。 */
  week_end_date: string
  /** "teaching"、"exam"、"break" 或 "unknown"。 */
  phase_type: PhaseType
  /** false 时前端显示阶段空状态，不展示课程。 */
  courses_visible: boolean
  items: WeekScheduleItem[]
}

/** 与后端 schedule::CalendarEvent / CalendarWeekResponse 对齐。 */
export interface CalendarEvent {
  /** 聚合来源：task 由 due_date 映射到全天/默认时段展示。 */
  kind: "course" | "exam" | "task"
  id: number
  title: string
  /** ISO weekday: 1 = 周一，7 = 周日。 */
  day_of_week: number
  /** 当天开始时间，格式 HH:mm。 */
  start_time: string
  /** 当天结束时间，格式 HH:mm。 */
  end_time: string
  location: string
  /** 展示颜色，由后端按来源统一决定。 */
  color: string
  /** 展示标签。课程为空数组；考试至少含"考试"+notes；任务按状态含"完成"/"截止"+解析的 tags。 */
  tags: string[]
  /** 点击事件后前端应该跳转的功能页。 */
  source_link: "todo" | "courses" | "exams"
}

export interface CalendarWeekResponse {
  /** 实际日历周对应的教学周序号，可能由 week_start_date 反推。 */
  week_index: number
  /** 学期标识，例如 2026S1。 */
  semester: string
  /** 学期锚点日期，格式 YYYY-MM-DD。 */
  semester_start_date: string
  /** 当前周周一日期，格式 YYYY-MM-DD。 */
  week_start_date: string
  /** 当前周周日日期，格式 YYYY-MM-DD。 */
  week_end_date: string
  /** "teaching"、"exam"、"break" 或 "unknown"。 */
  phase_type: PhaseType
  /** false 时课程事件已由后端隐藏。 */
  courses_visible: boolean
  events: CalendarEvent[]
}

/** get_calendar_week 命令入参。week_start_date 存在时优先按自然周定位。 */
export interface CalendarWeekCmd {
  semester: string
  week_index: number
  /** YYYY-MM-DD；省略时后端从课程或请求周推导。 */
  semester_start_date?: string
  /** YYYY-MM-DD；存在时后端会归一化到该日期所在周的周一。 */
  week_start_date?: string
}

export type PhaseType = "unknown" | "teaching" | "exam" | "break"
