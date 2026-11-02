import { Capacitor, registerPlugin } from "@capacitor/core"
import type { Palette, AppearanceMode } from "./presentation"
import type { UiStyle } from "./ui-style"

interface Appearance {
  setTheme(input: { theme: "dark" | "light" }): Promise<void>
  setAppearance(input: { theme: "dark" | "light"; style: UiStyle; palette: Palette; mode: AppearanceMode }): Promise<void>
}
const bridge = registerPlugin<Appearance>("XiqueAppearance")
let pending: Promise<void> = Promise.resolve()
export function syncSystemAppearance(theme: "dark" | "light", style: UiStyle = "classic", palette: Palette = "original", mode: AppearanceMode = theme) {
  if (!Capacitor.isPluginAvailable("XiqueAppearance")) return Promise.resolve()
  // Theme switches are ordered so a quick double tap cannot leave stale bars.
  pending = pending
    .catch(() => {})
    .then(() => bridge.setAppearance({ theme, style, palette, mode }).catch(() => bridge.setTheme({ theme })))
    .catch(() => {})
  return pending
}
