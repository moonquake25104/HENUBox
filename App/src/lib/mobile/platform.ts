import { Capacitor } from "@capacitor/core"

// A mobile build must fail closed even when its native bridge fails to load.
export const nativeApp =
  Capacitor.isNativePlatform() || import.meta.env.VITE_XIQUE_PLATFORM === "android"
const addressKey = "xique-native-api-origin"

export function normalizeServerOrigin(value: string): string {
  let url: URL
  try {
    url = new URL(value.trim())
  } catch {
    throw new Error("请输入完整的 HTTPS 服务地址。")
  }
  const host = url.hostname.toLowerCase().replace(/\.$/, "")
  if (
    url.protocol !== "https:" ||
    url.username ||
    url.password ||
    url.pathname !== "/" ||
    url.search ||
    url.hash ||
    ["localhost", "0.0.0.0", "[::1]"].includes(host) ||
    /^127\./.test(host)
  ) {
    throw new Error("请输入电脑或服务器可达的 HTTPS 地址，不带路径；手机不能使用 localhost。")
  }
  return url.origin
}
function initialNativeOrigin() {
  try {
    const saved = localStorage.getItem(addressKey)
    const supplied = saved ?? import.meta.env.VITE_API_BASE_URL
    return supplied ? normalizeServerOrigin(supplied) : ""
  } catch {
    return ""
  }
}
export let apiOrigin: string = nativeApp
  ? initialNativeOrigin()
  : (import.meta.env.VITE_API_BASE_URL ?? "")
export function storeNativeOrigin(value: string) {
  const origin = normalizeServerOrigin(value)
  localStorage.setItem(addressKey, origin) // Address only; never credentials.
  apiOrigin = origin
}
