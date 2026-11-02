import type {
  CreateTaskRequest,
  Task,
  TaskFilterParams,
  UpdateTaskRequest,
} from "@/types/task"
import { apiFetch, buildQuery } from "./client"
import type { StatusResponse } from "./client"

export function listTasks(filters?: TaskFilterParams): Promise<Task[]> {
  const query = buildQuery({
    status_filter: filters?.status_filter,
    priority_filter: filters?.priority_filter,
    sort_by: filters?.sort_by,
    sort_order: filters?.sort_order,
  })
  return apiFetch<Task[]>(`/api/tasks${query}`)
}

export function createTask(cmd: CreateTaskRequest): Promise<Task> {
  return apiFetch<Task>("/api/tasks", {
    method: "POST",
    body: JSON.stringify(cmd),
  })
}

export function updateTask(id: number, cmd: UpdateTaskRequest): Promise<Task> {
  return apiFetch<Task>(`/api/tasks/${id}`, {
    method: "PATCH",
    body: JSON.stringify(cmd),
  })
}

export function deleteTask(id: number): Promise<StatusResponse> {
  return apiFetch<StatusResponse>(`/api/tasks/${id}`, { method: "DELETE" })
}

export function completeDailyTask(id: number): Promise<StatusResponse> {
  return apiFetch<StatusResponse>(`/api/tasks/${id}/complete`, { method: "POST" })
}

export function uncompleteDailyTask(id: number): Promise<StatusResponse> {
  return apiFetch<StatusResponse>(`/api/tasks/${id}/uncomplete`, { method: "POST" })
}
