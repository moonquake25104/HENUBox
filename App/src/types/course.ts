/** 与后端 db::models::Course 对齐，是课程列表和周视图来源实体。 */
export interface Course {
  id: number
  /** 跨设备稳定标识（UUID）。同步合并键，不等于本地 SQLite id。 */
  sync_id: string
  name: string
  /** ISO weekday: 1 = 周一，7 = 周日。 */
  day_of_week: number
  /** 上课开始时间，格式 HH:mm，本地课程时间。 */
  start_time: string
  /** 上课结束时间，格式 HH:mm，本地课程时间。 */
  end_time: string
  /** 周次规则文本，例如 "1-17周全周"，由后端 schedule::matches_week_pattern 解释。 */
  week_pattern: string
  /** 学期锚点日期，格式 YYYY-MM-DD，用于计算当前教学周。 */
  semester_start_date: string
  location: string
  teacher: string
  /** 课程显示颜色，十六进制 RGB 字符串。 */
  color: string
  /** 学期标识，例如 2026S1。 */
  semester: string
  /** UTC ISO 8601 创建时间，由后端 db::chrono_now 生成。 */
  created_at: string
  /** UTC ISO 8601 更新时间。LWW 同步以此字段比较胜负（墓碑优先取 deleted_at）。 */
  updated_at: string
  /** 墓碑时间戳。正常列表查询只返回 null；非 null 表示已软删除并参与同步传播。 */
  deleted_at: string | null
}

/** create_course 命令入参；可省略字段由 Rust command 使用默认值补齐。 */
export interface CreateCourseRequest {
  name: string
  /** ISO weekday: 1 = 周一，7 = 周日。 */
  day_of_week: number
  /** HH:mm。 */
  start_time: string
  /** HH:mm。 */
  end_time: string
  week_pattern?: string
  /** YYYY-MM-DD。 */
  semester_start_date?: string
  location?: string
  teacher?: string
  color?: string
  semester?: string
}

/** update_course 命令入参；省略字段表示沿用当前课程值。 */
export interface UpdateCourseRequest {
  name?: string
  /** ISO weekday: 1 = 周一，7 = 周日。 */
  day_of_week?: number
  /** HH:mm。 */
  start_time?: string
  /** HH:mm。 */
  end_time?: string
  week_pattern?: string
  /** YYYY-MM-DD。 */
  semester_start_date?: string
  location?: string
  teacher?: string
  color?: string
  semester?: string
}

/** get_all_courses 过滤参数，字段名与 commands::courses::CourseFilterParams 对齐。 */
export interface CourseFilterParams {
  semester?: string | null
}
