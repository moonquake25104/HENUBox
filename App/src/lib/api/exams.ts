import type { CreateExamRequest, Exam, UpdateExamRequest } from "@/types/exam"
import type { ImportTextResult } from "@/types/course-import"
import { apiFetch } from "./client"
import type { StatusResponse } from "./client"

export interface ImportExamsFromTextRequest {
  text: string
  semester: string
}

export function listExams(): Promise<Exam[]> {
  return apiFetch<Exam[]>("/api/exams")
}

export function createExam(cmd: CreateExamRequest): Promise<Exam> {
  return apiFetch<Exam>("/api/exams", {
    method: "POST",
    body: JSON.stringify(cmd),
  })
}

export function updateExam(id: number, cmd: UpdateExamRequest): Promise<Exam> {
  return apiFetch<Exam>(`/api/exams/${id}`, {
    method: "PATCH",
    body: JSON.stringify(cmd),
  })
}

export function deleteExam(id: number): Promise<StatusResponse> {
  return apiFetch<StatusResponse>(`/api/exams/${id}`, { method: "DELETE" })
}

export function importExamsFromText(cmd: ImportExamsFromTextRequest): Promise<ImportTextResult> {
  return apiFetch<ImportTextResult>("/api/exams/import-text", {
    method: "POST",
    body: JSON.stringify(cmd),
  })
}
