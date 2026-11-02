/** 与后端 commands::briefing::TodayBriefingResponse 对齐。 */

import type { CurrentPhaseStatus } from "@/types/notification"

export interface NextCourse {
  title: string
  start_time: string
  end_time: string
  location: string
}

export interface TodayCourses {
  today_count: number
  current_course: NextCourse | null
  next_course: NextCourse | null
}

export interface TaskSpotlight {
  id: number
  title: string
  priority: "high" | "medium" | "low"
  due_date: string | null
}

export interface TodayTasks {
  overdue_count: number
  due_today_count: number
  spotlight: TaskSpotlight[]
  /** 今日未完成的每日任务数（习惯类任务）。 */
  daily_unfinished_count: number
  /** 今日未完成的每日任务（按优先级/标题排序，最多 3 条）。 */
  daily_spotlight: TaskSpotlight[]
}

export interface UpcomingExam {
  id: number
  course_name: string
  exam_datetime: string
  days_until: number
  location: string
}

export interface PomodoroBriefing {
  is_running: boolean
  phase: "work" | "short_break" | "long_break"
  remaining_seconds: number
  completed_sessions: number
}

export type PhaseBriefing = Pick<
  CurrentPhaseStatus,
  | "phase_type"
  | "term_label"
  | "current_week"
  | "courses_visible"
  | "exam_notifications_enabled"
  | "pomodoro_profile"
>

/** get_today_briefing 命令的完整响应。 */
export interface TodayBriefingResponse {
  date: string
  weekday_label: string
  courses: TodayCourses
  tasks: TodayTasks
  exam: UpcomingExam | null
  pomodoro: PomodoroBriefing
  phase: PhaseBriefing
}
