import { Capacitor, registerPlugin } from "@capacitor/core"
import { apiOrigin, nativeApp, normalizeServerOrigin, storeNativeOrigin } from "./platform"
import { forgetOfflineTimetable, restoreOfflineTimetable } from "./offline-timetable"

interface NativeCredentials {
  server_origin: string
  account_id: string
  access_token: string
}
interface NativeRead {
  generation: number
  session: NativeCredentials | null
}
interface NativeSessionPlugin {
  read(): Promise<NativeRead>
  write(value: NativeCredentials & { expected_generation: number }): Promise<{ generation: number }>
  clear(): Promise<{ generation: number }>
}
const bridge = registerPlugin<NativeSessionPlugin>("XiqueSession")
const tokenKey = "kairos_token"
const resetKey = "xique-native-session-reset-required"
let memoryToken: string | null = null
let account: string | null = null
let epoch = 0
let nativeGeneration = 0
let ready = !nativeApp
let bootstrap: Promise<void> | null = null

function generation(value: unknown): value is number {
  return typeof value === "number" && Number.isSafeInteger(value) && value >= 0
}
function storageError() {
  return new Error("本机安全登录存储暂不可用，请重试或清除本机登录。")
}
function reportStorageFailure() {
  window.dispatchEvent(new Event("xique:session-storage-error"))
}
function ensureBridge() {
  if (!Capacitor.isNativePlatform() || !Capacitor.isPluginAvailable("XiqueSession")) {
    throw new Error("这个安装包的安全登录组件尚未就绪，请使用完成原生接入的版本。")
  }
}
async function clearNative() {
  ensureBridge()
  const result = await bridge.clear()
  if (!generation(result.generation)) throw storageError()
  nativeGeneration = result.generation
  ready = true
  localStorage.removeItem(resetKey)
}
export const sessionEpoch = () => epoch
export const nativeAccount = () => account
export function assertSession(current: number) {
  if (current !== epoch) throw new Error("登录状态已变化，请重新操作。")
}
export function getToken(): string | null {
  if (nativeApp) return memoryToken
  try {
    return localStorage.getItem(tokenKey)
  } catch {
    return null
  }
}
export function initializeSession(): Promise<void> {
  if (!nativeApp) return Promise.resolve()
  if (bootstrap) return bootstrap
  const current = epoch
  bootstrap = (async () => {
    // Never migrate a WebView localStorage token into native secure storage.
    localStorage.removeItem(tokenKey)
    ensureBridge()
    if (localStorage.getItem(resetKey)) await clearNative()
    let value: NativeRead
    try {
      value = await bridge.read()
    } catch {
      throw storageError()
    }
    assertSession(current)
    if (!value || !generation(value.generation)) throw storageError()
    nativeGeneration = value.generation
    const saved = value.session
    if (
      saved !== null &&
      (!saved ||
        typeof saved.account_id !== "string" ||
        !saved.account_id.trim() ||
        typeof saved.access_token !== "string" ||
        !saved.access_token.trim() ||
        typeof saved.server_origin !== "string")
    )
      throw storageError()
    if (saved) {
      let canonical: string
      try {
        canonical = normalizeServerOrigin(saved.server_origin)
      } catch {
        throw storageError()
      }
      // A restored token can only ever go back to its original configured server.
      if (canonical === apiOrigin) {
        await restoreOfflineTimetable(nativeGeneration)
        assertSession(current)
        memoryToken = saved.access_token
        account = saved.account_id
      }
    }
    ready = true
  })().catch((err) => {
    if (current === epoch) {
      memoryToken = null
      account = null
      ready = false
      bootstrap = null
    }
    throw err
  })
  return bootstrap
}
export async function setToken(token: string, username?: string) {
  if (!token.trim()) throw new Error("登录响应缺少会话，请重新登录。")
  if (!nativeApp) {
    localStorage.setItem(tokenKey, token)
    epoch++
    return
  }
  if (!ready || !apiOrigin || !username?.trim()) throw storageError()
  ensureBridge()
  const current = epoch
  const expected = nativeGeneration
  let result: { generation: number }
  try {
    result = await bridge.write({
      expected_generation: expected,
      server_origin: apiOrigin,
      account_id: username.trim(),
      access_token: token,
    })
  } catch {
    assertSession(current)
    throw storageError()
  }
  assertSession(current)
  if (!generation(result.generation) || result.generation <= expected) throw storageError()
  nativeGeneration = result.generation
  await restoreOfflineTimetable(nativeGeneration)
  assertSession(current)
  memoryToken = token
  account = username.trim()
  epoch++
}
export async function clearToken() {
  epoch++
  forgetOfflineTimetable()
  memoryToken = null
  account = null
  try {
    localStorage.removeItem(tokenKey)
  } catch {
    /* Never use a web token in native mode. */
  }
  // Hide the old workspace immediately, even if persistence is slow or fails.
  window.dispatchEvent(new Event("kairos:unauthorized"))
  if (!nativeApp) return
  ready = false
  try {
    localStorage.setItem(resetKey, "1") // Non-secret crash recovery marker.
    await clearNative()
    bootstrap = Promise.resolve()
  } catch {
    ready = false
    bootstrap = null
    reportStorageFailure()
    throw storageError()
  }
}
export async function changeNativeServer(value: string) {
  const canonical = normalizeServerOrigin(value)
  if (canonical === apiOrigin) return
  // Clear the previous account before publishing a different request target.
  await clearToken()
  storeNativeOrigin(canonical)
  window.dispatchEvent(new Event("xique:server-changed"))
}
