import { registerPlugin } from "@capacitor/core"
import type { Course } from "@/types/course"
import type { SemesterSettings } from "@/lib/api/academic"

export type LocalCourseInput = Pick<Course, "name" | "day_of_week" | "start_time" | "end_time" | "week_pattern" | "semester_start_date" | "location" | "teacher" | "color" | "semester">

// Android local-v2 contract. No desktop JWT, endpoint, cookies or userCode
// cross this bridge. Activation in App.tsx waits for the M2 native delivery.
export interface LocalBootstrap {
  schema_version: 2
  workspace_id: string
  revision: number
  mode: "LOCAL"
  saved_at: string | null
  courses: Course[]
  legacy: { state: "NONE" | "PRESERVED" | "ADOPTED"; course_count: number }
  capabilities: { henu_direct: boolean; can_commit: boolean; can_resolve: boolean }
  migration_warning?: string
}
export interface LocalOperation {
  operation_id: string
  kind: "LOGIN" | "FETCH"
  state: "WAITING_FOR_USER" | "VERIFYING" | "QUEUED" | "RUNNING" | "SUCCEEDED" | "FAILED" | "CANCELLED" | "EXPIRED"
  expires_at: string
  preview_id: string | null
  error: { code: string; message: string } | null
}
export interface LocalConnection {
  connection_state: "DISCONNECTED" | "CONNECTED" | "AUTH_REQUIRED" | "TEMPORARY_FAILURE" | "UNAVAILABLE"
  connection_generation: number
  capabilities: { interactive_login: boolean; direct_fetch: boolean; can_commit: boolean; can_resolve: boolean }
  active_login_id: string | null
  active_job_id: string | null
  current_term: { upstream_year: string; upstream_term: string; label: string } | null
  last_checked_at: string | null
}
export interface LocalPreview {
  preview_id: string
  semester: string
  semester_label: string
  base_revision: number
  connection_generation: number
  expires_at: string
  status: "INVALID" | "NEEDS_CONFIGURATION" | "NEEDS_REVIEW" | "READY" | "COMMITTED"
  can_commit: boolean
  validation: { state: "FRESH" | "STALE" }
  stats: { logical_courses: number; sessions: number; source_rows: number; unparsed_fragments: number }
  issues: { code: string; severity: "WARNING" | "BLOCKING"; message: string; preview_course_id?: string | null }[]
  diff: {
    added: string[]; unchanged: string[]; changed: unknown[]; missing: unknown[]
    legacy_link_candidates: unknown[]; manual_kept: unknown[]
  }
  courses: {
    preview_course_id: string; name: string; upstream_course_code?: string | null
    sessions: { preview_session_id: string; weekday: number; sections: number[]; weeks: number[]; location: string; teacher: string; start_time: string | null; end_time: string | null }[]
  }[]
}
export interface LocalReceipt {
  commit_id: string; preview_id: string; semester: string; revision: number
  no_changes: boolean; courses_created: number; sessions_created: number
  legacy_linked: number; manual_kept: number; projection_created: number
  projection_updated: number; unchanged: number; warning_codes: string[]; committed_at: string
}
interface WorkspaceInput { workspace_id: string }
export interface LocalApi {
  saveCourse(input: WorkspaceInput & { expected_revision: number; course_id: number; sync_id: string; course: LocalCourseInput }): Promise<{ revision: number; course_id: number }>
  deleteCourse(input: WorkspaceInput & { expected_revision: number; course_id: number }): Promise<{ revision: number }>
  bootstrap(): Promise<LocalBootstrap>
  getSettings(input: WorkspaceInput & { semester: string }): Promise<{ revision: number; settings: SemesterSettings }>
  putSettings(input: WorkspaceInput & { expected_revision: number; settings: SemesterSettings }): Promise<{ revision: number; settings: SemesterSettings }>
  setCourseColor(input: WorkspaceInput & { course_id: number; color: string }): Promise<{ revision: number; color: string }>
}
export interface SchoolApi {
  getConnection(input: WorkspaceInput): Promise<LocalConnection>
  startLogin(input: WorkspaceInput & { idempotency_key: string }): Promise<LocalOperation>
  verifyLogin(input: WorkspaceInput & { operation_id: string }): Promise<LocalOperation>
  getOperation(input: WorkspaceInput & { operation_id: string }): Promise<LocalOperation>
  cancelOperation(input: WorkspaceInput & { operation_id: string }): Promise<{ cancelled: boolean }>
  fetchPreview(input: WorkspaceInput & { semester: string; expected_revision: number; idempotency_key: string }): Promise<LocalOperation>
  getPreview(input: WorkspaceInput & { preview_id: string }): Promise<LocalPreview>
  commitPreview(input: WorkspaceInput & { preview_id: string; expected_revision: number; idempotency_key: string; acknowledged_warning_codes: string[] }): Promise<LocalReceipt>
  disconnect(input: WorkspaceInput & { expected_connection_generation: number }): Promise<{ connection_generation: number; connection_state: "DISCONNECTED" }>
}
export const localWorkspaceBridge = registerPlugin<LocalApi>("XiqueLocal")
export const localSchoolBridge = registerPlugin<SchoolApi>("XiqueHenu")
export const operationActive = (op: LocalOperation | null) => Boolean(op && ["WAITING_FOR_USER", "VERIFYING", "QUEUED", "RUNNING"].includes(op.state))

const errors: Record<string, string> = {
  INVALID_ARGUMENT: "请检查课程时间、1–18周的周次或开学日期；已有课表仍保留。",
  REVISION_CONFLICT: "本机课表或设置已变化，请重新检查更新。",
  PREVIEW_EXPIRED: "这份预览已过期，请重新检查更新。",
  UPSTREAM_AUTH_REQUIRED: "学校需要重新登录，本机课表仍可查看。",
  UPSTREAM_ACCOUNT_CHANGED: "本次学校账号与原账号不同，已保留本机课表。",
  UPSTREAM_TLS_ERROR: "学校安全连接失败，本机课表已保留。",
  UPSTREAM_UNAVAILABLE: "暂时无法连接学校，本机课表已保留。",
  OPERATION_IN_PROGRESS: "已有任务进行中，请等待或取消。",
  UNSUPPORTED_DEVICE: "此设备暂不支持手机直接登录，已保存的课表仍可查看。",
  STORAGE_UNAVAILABLE: "手机存储暂不可用，请重试。",
  REVIEW_UNSUPPORTED: "发现课表变化，需要进一步核对；当前本机课表保持原样。",
}
export function localSchoolError(error: unknown): string {
  const code = error && typeof error === "object" && "code" in error ? String(error.code) : ""
  return errors[code] ?? "操作未完成，请重试；原有本机课表已保留。"
}

export function maySavePreview(preview: LocalPreview | null, revision: number, generation: number | undefined, checked: boolean, acknowledgements: string[], now = Date.now()) {
  return Boolean(preview && checked && preview.can_commit && preview.status === "READY" &&
    preview.validation.state === "FRESH" && preview.base_revision === revision &&
    preview.connection_generation === generation && Date.parse(preview.expires_at) > now &&
    preview.stats.sessions > 0 && preview.stats.unparsed_fragments === 0 &&
    preview.diff.changed.length === 0 && preview.diff.missing.length === 0 &&
    acknowledgements.every(code => preview.issues.some(issue => issue.severity === "WARNING" && issue.code === code)) &&
    preview.issues.every(issue => issue.severity === "WARNING" && acknowledgements.includes(issue.code)))
}

export function unscheduledCourses(preview: LocalPreview | null) {
  const ids = new Set(preview?.issues.filter(i => i.code === "UNSCHEDULED_COURSE" && i.severity === "WARNING").map(i => i.preview_course_id) ?? [])
  return preview?.courses.filter(c => ids.has(c.preview_course_id) && c.sessions.length === 0) ?? []
}
