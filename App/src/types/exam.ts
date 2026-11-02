/** 与后端 db::models::Exam 对齐；days_until 是前端列表派生字段。 */
export interface Exam {
  id: number
  /** 跨设备稳定标识（UUID）。同步合并键，不等于本地 SQLite id。 */
  sync_id: string
  course_name: string
  /** 考试开始时间，RFC3339 字符串；前端按本地时区展示。 */
  exam_datetime: string
  /** 考试结束时间，RFC3339 字符串；空字符串表示未提供结束时间。 */
  exam_end_datetime: string
  location: string
  notes: string
  /** 关联课程的本地 SQLite id；null 表示未绑定课程。 */
  course_id: number | null
  /** 学期标识，例如 2026S1。 */
  semester: string
  /** UTC ISO 8601 创建时间，由后端 db::chrono_now 生成。 */
  created_at: string
  /** UTC ISO 8601 更新时间。LWW 同步以此字段比较胜负（墓碑优先取 deleted_at）。 */
  updated_at: string
  /** 墓碑时间戳。正常列表查询只返回 null；非 null 表示已软删除并参与同步传播。 */
  deleted_at: string | null
  /** 前端根据 exam_datetime 计算的展示字段，后端不会返回。 */
  days_until?: number
}

/** create_exam 命令入参；可省略字段由 Rust command 使用默认值补齐。 */
export interface CreateExamRequest {
  course_name: string
  /** RFC3339 字符串。 */
  exam_datetime: string
  /** RFC3339 字符串；省略时后端保存为空字符串。 */
  exam_end_datetime?: string
  location?: string
  notes?: string
  course_id?: number | null
  semester?: string
}

/** update_exam 命令入参；省略字段表示沿用当前考试值。 */
export interface UpdateExamRequest {
  course_name?: string
  /** RFC3339 字符串。 */
  exam_datetime?: string
  /** RFC3339 字符串；空字符串表示清空结束时间。 */
  exam_end_datetime?: string
  location?: string
  notes?: string
  course_id?: number | null
  semester?: string
}
