import type {
  Course,
  CourseFilterParams,
  CreateCourseRequest,
  UpdateCourseRequest,
} from "@/types/course"
import type { ImportTextResult } from "@/types/course-import"
import type { WeekScheduleResponse } from "@/types/schedule"
import { apiFetch, buildQuery } from "./client"
import type { StatusResponse } from "./client"

export interface WeekScheduleCmd {
  semester: string
  week_index: number
  semester_start_date?: string
}

export interface ImportCoursesFromTextRequest {
  text: string
  semester: string
  semester_start_date: string
}

export interface ResetSemesterDatesResponse {
  updated: number
}

export function listCourses(filters?: CourseFilterParams): Promise<Course[]> {
  const query = buildQuery({ semester: filters?.semester })
  return apiFetch<Course[]>(`/api/courses${query}`)
}

export function getWeekSchedule(cmd: WeekScheduleCmd): Promise<WeekScheduleResponse> {
  const query = buildQuery({
    semester: cmd.semester,
    week_index: cmd.week_index,
    semester_start_date: cmd.semester_start_date,
  })
  return apiFetch<WeekScheduleResponse>(`/api/calendar/week${query}`)
}

export function createCourse(cmd: CreateCourseRequest): Promise<Course> {
  return apiFetch<Course>("/api/courses", {
    method: "POST",
    body: JSON.stringify(cmd),
  })
}

export function updateCourse(id: number, cmd: UpdateCourseRequest): Promise<Course> {
  return apiFetch<Course>(`/api/courses/${id}`, {
    method: "PATCH",
    body: JSON.stringify(cmd),
  })
}

export function deleteCourse(id: number): Promise<StatusResponse> {
  return apiFetch<StatusResponse>(`/api/courses/${id}`, { method: "DELETE" })
}

export function resetSemesterDates(date: string): Promise<ResetSemesterDatesResponse> {
  return apiFetch<ResetSemesterDatesResponse>("/api/courses/reset-semester-dates", {
    method: "POST",
    body: JSON.stringify({ date }),
  })
}

export function importCoursesFromText(cmd: ImportCoursesFromTextRequest): Promise<ImportTextResult> {
  return apiFetch<ImportTextResult>("/api/courses/import-text", {
    method: "POST",
    body: JSON.stringify(cmd),
  })
}
