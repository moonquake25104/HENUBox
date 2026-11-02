import { apiFetch, buildQuery } from "./client"

export interface Period {
  index: number
  start_time: string | null
  end_time: string | null
  configured: boolean
}
export interface SemesterSettings {
  campus?: "郑州" | "金明" | "明伦" | null
  semester: string
  revision: number
  semester_label: string | null
  week_anchor: { start_date: string | null; source: string; confirmed: boolean }
  total_weeks: number | null
  period_count: number
  periods: Period[]
}
export interface AcademicPreview {
  preview_id: string
  acquisition?: "FILE_IMPORT" | "DIRECT_FETCH"
  source_obtained_at?: string | null
  commit_id?: string
  base_versions?: { aggregate_revision: number }
  diff?: {
    added: string[]
    changed: { preview_session_id: string; session_id: string; changed_fields: string[] }[]
    unchanged: string[]
    missing: {
      session_id: string
      academic_course_id: string
      decision: "RETAIN" | "RETIRE" | null
    }[]
    ambiguous: { preview_session_id: string; candidate_session_ids: string[] }[]
  }
  source_semester_label: string | null
  status: string
  can_commit: boolean
  expires_at: string
  validation?: { state: "FRESH" | "STALE"; current_aggregate_revision: number }
  summary: {
    logical_courses: number
    sessions: number
    source_rows: number
    unparsed_fragments: number
  }
  issues: {
    code: string
    severity: "WARNING" | "BLOCKING"
    message: string
    preview_course_id?: string
  }[]
  courses: {
    preview_course_id: string
    academic_course_id?: string | null
    name: string
    credits: string | null
    sessions: {
      preview_session_id: string
      weekday: number
      sections: number[]
      weeks: number[]
      location: string | null
      teacher: string | null
      start_time: string | null
      end_time: string | null
    }[]
  }[]
}

export interface AcademicResolution {
  kind:
    | "LINK_COURSE"
    | "CREATE_COURSE"
    | "LINK_SESSION"
    | "CREATE_SESSION"
    | "RETAIN_SESSION"
    | "RETIRE_SESSION"
  preview_course_id?: string
  preview_session_id?: string
  academic_course_id?: string
  session_id?: string
}
export interface AcademicReceipt {
  commit_id: string
  preview_id: string
  semester: string
  committed_at: string
  revision: number
  aggregate_revision: number
  no_changes: boolean
  counts: {
    sessions_created: number
    sessions_updated: number
    sessions_retired: number
    unchanged: number
    retained: number
  }
}
export interface AcademicTimetable {
  aggregate_revision: number
  courses: {
    id: string
    name: string
    sessions: {
      session_id: string
      active: boolean
      source_values: {
        weekday: number
        sections: number[]
        weeks: number[]
        location: string | null
      }
    }[]
  }[]
}
export const getAcademicTimetable = (semester: string) =>
  apiFetch<AcademicTimetable>(`/api/academic/timetable${buildQuery({ semester })}`)
export const rebaseAcademicPreview = async (
  id: string,
  revision: number,
  resolutions: AcademicResolution[],
  confirm: boolean,
) => {
  const value = await apiFetch<AcademicPreview>(
    `/api/academic/henu/previews/${encodeURIComponent(id)}/rebase`,
    {
      method: "POST",
      body: JSON.stringify({
        expected_aggregate_revision: revision,
        resolutions,
        confirm_semester_mapping: confirm,
      }),
    },
  )
  return getAcademicPreview(value.preview_id)
}
export const commitAcademicPreview = (
  id: string,
  body: {
    expected_aggregate_revision: number
    idempotency_key: string
    acknowledged_warning_codes: string[]
  },
) =>
  apiFetch<AcademicReceipt>(`/api/academic/henu/previews/${encodeURIComponent(id)}/commit`, {
    method: "POST",
    body: JSON.stringify(body),
  })
export const getAcademicReceipt = (id: string) =>
  apiFetch<AcademicReceipt>(`/api/academic/commits/${encodeURIComponent(id)}`)
export const getAcademicSettings = (semester: string) =>
  apiFetch<SemesterSettings>(`/api/academic/semester-settings${buildQuery({ semester })}`)
export const saveAcademicSettings = (value: SemesterSettings) =>
  apiFetch<SemesterSettings>("/api/academic/semester-settings", {
    method: "PUT",
    body: JSON.stringify({
      semester: value.semester,
      semester_label: value.semester_label,
      week_anchor: value.week_anchor,
      total_weeks: value.total_weeks,
      period_count: value.period_count,
      periods: value.periods,
      expected_revision: value.revision,
      week_start_day: 1,
    }),
  })
export const getAcademicPreview = (id: string) =>
  apiFetch<AcademicPreview>(`/api/academic/henu/previews/${encodeURIComponent(id)}`)
export async function uploadAcademicPreview(semester: string, file: File) {
  const body = new FormData()
  body.set("semester", semester)
  body.set("file", file)
  const created = await apiFetch<AcademicPreview>("/api/academic/henu/previews", {
    method: "POST",
    body,
  })
  return getAcademicPreview(created.preview_id)
}
