import { apiFetch } from "./client"

export interface PushConfig {
  enabled: boolean
  public_key: string
  course_reminders: boolean
}
export interface PushDevice {
  id: number
  active: boolean
}
export interface PushTest {
  id: number
  subscription_id: number
  due_at: string
  status: string
  detail: string
  confirmed: boolean
}
export const pushConfig = () => apiFetch<PushConfig>("/api/push/config")
export const savePushDevice = (subscription: PushSubscriptionJSON) =>
  apiFetch<PushDevice>("/api/push/subscriptions", {
    method: "POST",
    body: JSON.stringify(subscription),
  })
export const pushDevice = (id: number) => apiFetch<PushDevice>(`/api/push/subscriptions/${id}`)
export const disablePushDevice = (id: number) =>
  apiFetch(`/api/push/subscriptions/${id}`, { method: "DELETE" })
export const pushTests = () => apiFetch<PushTest[]>("/api/push/tests")
export const sendPushTest = (id: number, delay: number) =>
  apiFetch<PushTest>("/api/push/test", {
    method: "POST",
    body: JSON.stringify({ subscription_id: id, delay_seconds: delay }),
  })
export const confirmPushTest = (id: number) =>
  apiFetch(`/api/push/tests/${id}/confirm`, { method: "POST" })

const storageKey = "xique-push-device"
export function storedPushDevice(): number | null {
  const id = Number(localStorage.getItem(storageKey))
  return Number.isSafeInteger(id) && id > 0 ? id : null
}
export async function detachPushDevice() {
  const id = storedPushDevice()
  if (id) await disablePushDevice(id)
  if ("serviceWorker" in navigator) {
    const registration = await navigator.serviceWorker.getRegistration("/")
    const subscription = await registration?.pushManager.getSubscription()
    if (subscription) await subscription.unsubscribe()
  }
  localStorage.removeItem(storageKey)
}
export function rememberPushDevice(id: number) {
  localStorage.setItem(storageKey, String(id))
}
