import { apiFetch } from "./client"
import type { StatusResponse } from "./client"

export interface AuthUser {
  username: string
}

export interface LoginResponse {
  token: string
  user: AuthUser
}

export interface MeResponse {
  username: string
}

export function login(username: string, password: string): Promise<LoginResponse> {
  return apiFetch<LoginResponse>("/api/auth/login", {
    method: "POST",
    body: JSON.stringify({ username, password }),
  })
}

export function logout(): Promise<StatusResponse> {
  return apiFetch<StatusResponse>("/api/auth/logout", { method: "POST" })
}

export function me(): Promise<MeResponse> {
  return apiFetch<MeResponse>("/api/auth/me")
}
