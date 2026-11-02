import { apiOrigin, nativeApp } from "../mobile/platform"
import { assertSession, clearToken, getToken, sessionEpoch } from "../mobile/session"
import { offlineTimetable } from "../mobile/offline-timetable"
export { apiOrigin as BASE } from "../mobile/platform"
export { getToken, setToken, clearToken } from "../mobile/session"

export interface StatusResponse {
  status: string
}

export class ApiRequestError extends Error {
  status: number
  constructor(message: string, status: number) {
    super(message)
    this.name = "ApiRequestError"
    this.status = status
  }
}

export type QueryValue = string | number | boolean | null | undefined

export function buildQuery(params: Record<string, QueryValue>): string {
  const search = new URLSearchParams()
  for (const [key, value] of Object.entries(params)) {
    if (value === null || value === undefined) continue
    const text = String(value).trim()
    if (text === "") continue
    search.set(key, text)
  }
  const qs = search.toString()
  return qs ? `?${qs}` : ""
}

function statusFallbackMessage(status: number): string {
  switch (status) {
    case 400:
      return "请求无效"
    case 403:
      return "没有权限执行此操作"
    case 404:
      return "请求的资源不存在"
    case 409:
      return "请求与当前状态冲突"
    case 500:
      return "服务器内部错误"
    case 502:
      return "上游服务不可用"
    default:
      return `请求失败（HTTP ${status}）`
  }
}

export async function readErrorMessage(res: Response): Promise<string> {
  try {
    const data: unknown = await res.json()
    if (data !== null && typeof data === "object" && "error" in data) {
      const value = (data as { error: unknown }).error
      if (typeof value === "string" && value.trim() !== "") {
        return value
      }
    }
  } catch {
    return statusFallbackMessage(res.status)
  }
  return statusFallbackMessage(res.status)
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const current = sessionEpoch()
  if (!path.startsWith("/api/")) throw new Error("API 路径无效")
  if (nativeApp && !apiOrigin) throw new Error("请先保存HENUBox 服务的 HTTPS 地址。")
  if (nativeApp && navigator.onLine === false && init?.method && init.method !== "GET") {
    throw new Error("当前离线，暂时无法提交，请联网后再试。")
  }
  const headers = new Headers(init?.headers)
  if (
    init?.body !== undefined &&
    !(init.body instanceof FormData) &&
    !headers.has("Content-Type")
  ) {
    headers.set("Content-Type", "application/json")
  }
  const token = getToken()
  if (token) {
    headers.set("Authorization", `Bearer ${token}`)
  }

  let res: Response
  try {
    res = await fetch(`${apiOrigin}${path}`, {
      ...init,
      headers,
      redirect: "error",
      ...(nativeApp && !init?.signal ? { signal: AbortSignal.timeout(10000) } : {}),
      ...(nativeApp ? { credentials: "omit" as const } : {}),
    })
  } catch (err) {
    if (nativeApp && err instanceof TypeError)
      throw new Error("无法连接HENUBox 服务，请检查网络、HTTPS 地址和证书配置。", { cause: err })
    throw err
  }
  // An old request (including an old 401) must never affect a newer account.
  assertSession(current)

  if (res.status === 401 && path === "/api/auth/login") {
    throw new Error("用户名或密码错误")
  }
  if (res.status === 401) {
    // Expired remote authorization must not revoke an already saved local timetable.
    if (nativeApp && offlineTimetable()) throw new ApiRequestError("在线登录已过期，本机课表仍可查看；更新前请重新登录。", 401)
    await clearToken()
    throw new Error("登录已过期，请重新登录")
  }
  if (!res.ok) {
    const message = await readErrorMessage(res)
    assertSession(current)
    throw new ApiRequestError(message, res.status)
  }
  if (res.status === 204) {
    return undefined as T
  }
  const body = (await res.json()) as T
  assertSession(current)
  return body
}
