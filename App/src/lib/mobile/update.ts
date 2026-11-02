import { Capacitor, registerPlugin } from "@capacitor/core"

export const REPOSITORY_URL = "https://github.com/moonquake25104/HENUBox"
export const RELEASES_URL = `${REPOSITORY_URL}/releases`
export const LATEST_RELEASE_URL = `${RELEASES_URL}/latest`
const LATEST_API = "https://api.github.com/repos/moonquake25104/HENUBox/releases/latest"

export type UpdateErrorCode =
  | "NO_RELEASE"
  | "RATE_LIMITED"
  | "NETWORK"
  | "INVALID_RELEASE"
  | "UPDATE_CHECK_FAILED"

export class UpdateCheckError extends Error {
  readonly code: UpdateErrorCode
  constructor(code: UpdateErrorCode, message: string) {
    super(message)
    this.name = "UpdateCheckError"
    this.code = code
  }
}

interface NativeRelease {
  current_version: string
  tag_name: string
  name: string
  body: string
  html_url: string
  published_at: string
  apk_url: string | null
}

interface XiqueUpdatePlugin {
  check(): Promise<NativeRelease>
  openRelease(options: { url: string }): Promise<void>
}

const nativePlugin = registerPlugin<XiqueUpdatePlugin>("XiqueUpdate")

export interface UpdateResult {
  currentVersion: string
  latestVersion: string
  latestTag: string
  releaseUrl: string
  apkUrl: string | null
  publishedAt: string
  notes: string
  updateAvailable: boolean
}

export function parseVersion(value: string): number[] | null {
  const match = /^[vV]?(\d+)\.(\d+)\.(\d+)(?:[-+][0-9A-Za-z.-]+)?$/.exec(value.trim())
  if (!match) return null
  const parts = [Number(match[1]), Number(match[2]), Number(match[3])]
  return parts.every(Number.isSafeInteger) ? parts : null
}

export function isNewerVersion(latest: string, current: string): boolean {
  const next = parseVersion(latest)
  const now = parseVersion(current)
  if (!next || !now) return false
  for (let i = 0; i < 3; i++) {
    if (next[i] !== now[i]) return next[i] > now[i]
  }
  return false
}

export function safeReleaseUrl(value: string | null | undefined): string {
  if (typeof value !== "string" || value.length > 2048) return LATEST_RELEASE_URL
  if (value === RELEASES_URL || value === LATEST_RELEASE_URL) return value
  return value.startsWith(`${RELEASES_URL}/`) ? value : LATEST_RELEASE_URL
}

function apkFromAssets(value: unknown): string | null {
  if (!Array.isArray(value)) return null
  for (const item of value) {
    if (!item || typeof item !== "object") continue
    const asset = item as Record<string, unknown>
    const name = typeof asset.name === "string" ? asset.name.toLowerCase() : ""
    const url = typeof asset.browser_download_url === "string" ? asset.browser_download_url : ""
    if (name.endsWith(".apk") && url.startsWith(`${RELEASES_URL}/download/`)) return url
  }
  return null
}

async function fetchLatestRelease(): Promise<NativeRelease> {
  let response: Response
  try {
    response = await fetch(LATEST_API, {
      headers: { Accept: "application/vnd.github+json", "X-GitHub-Api-Version": "2022-11-28" },
      cache: "no-store",
      signal: AbortSignal.timeout(10000),
    })
  } catch {
    throw new UpdateCheckError("NETWORK", "无法连接 GitHub，请检查网络后重试。")
  }
  if (response.status === 404) {
    throw new UpdateCheckError("NO_RELEASE", "仓库还没有发布版本。")
  }
  if (response.status === 403 || response.status === 429) {
    throw new UpdateCheckError("RATE_LIMITED", "GitHub 请求过于频繁，请稍后再试。")
  }
  if (!response.ok) {
    throw new UpdateCheckError("UPDATE_CHECK_FAILED", "GitHub 暂时无法返回版本信息。")
  }
  let data: unknown
  try {
    data = await response.json()
  } catch {
    throw new UpdateCheckError("INVALID_RELEASE", "GitHub 返回的版本信息不完整。")
  }
  if (!data || typeof data !== "object") {
    throw new UpdateCheckError("INVALID_RELEASE", "GitHub 返回的版本信息不完整。")
  }
  const record = data as Record<string, unknown>
  return {
    current_version: __APP_VERSION__,
    tag_name: typeof record.tag_name === "string" ? record.tag_name : "",
    name: typeof record.name === "string" ? record.name : "",
    body: typeof record.body === "string" ? record.body : "",
    html_url: typeof record.html_url === "string" ? record.html_url : "",
    published_at: typeof record.published_at === "string" ? record.published_at : "",
    apk_url: apkFromAssets(record.assets),
  }
}

function normalizeRelease(raw: NativeRelease): UpdateResult {
  const latestTag = raw.tag_name.trim()
  const latestVersion = latestTag.replace(/^[vV]/, "")
  if (!parseVersion(latestVersion)) {
    throw new UpdateCheckError("INVALID_RELEASE", "最新版本号无法识别。")
  }
  const currentVersion = raw.current_version.trim() || "0.0.0"
  return {
    currentVersion,
    latestVersion,
    latestTag,
    releaseUrl: safeReleaseUrl(raw.html_url),
    apkUrl: raw.apk_url && raw.apk_url.startsWith(`${RELEASES_URL}/download/`) ? raw.apk_url : null,
    publishedAt: raw.published_at,
    notes: raw.body.slice(0, 6000),
    updateAvailable: isNewerVersion(latestVersion, currentVersion),
  }
}

function asUpdateError(error: unknown): UpdateCheckError {
  if (error instanceof UpdateCheckError) return error
  const code =
    error && typeof error === "object" && "code" in error
      ? String((error as { code: unknown }).code)
      : ""
  switch (code) {
    case "NO_RELEASE":
      return new UpdateCheckError("NO_RELEASE", "仓库还没有发布版本。")
    case "RATE_LIMITED":
      return new UpdateCheckError("RATE_LIMITED", "GitHub 请求过于频繁，请稍后再试。")
    case "NETWORK":
      return new UpdateCheckError("NETWORK", "无法连接 GitHub，请检查网络后重试。")
    case "INVALID_RELEASE":
      return new UpdateCheckError("INVALID_RELEASE", "GitHub 返回的版本信息不完整。")
    default:
      return new UpdateCheckError("UPDATE_CHECK_FAILED", "暂时无法检查更新，请稍后重试。")
  }
}

export async function checkForUpdate(): Promise<UpdateResult> {
  let raw: NativeRelease
  try {
    raw =
      Capacitor.isNativePlatform() && Capacitor.isPluginAvailable("XiqueUpdate")
        ? await nativePlugin.check()
        : await fetchLatestRelease()
  } catch (error) {
    throw asUpdateError(error)
  }
  return normalizeRelease(raw)
}

export async function openReleasePage(url: string): Promise<void> {
  const safe = safeReleaseUrl(url)
  if (Capacitor.isNativePlatform() && Capacitor.isPluginAvailable("XiqueUpdate")) {
    await nativePlugin.openRelease({ url: safe })
    return
  }
  window.open(safe, "_blank", "noopener,noreferrer")
}
