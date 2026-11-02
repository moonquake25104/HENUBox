import { useEffect, useLayoutEffect, useSyncExternalStore } from "react"
import { applyTheme, useTheme } from "@/hooks/use-theme"
import palettes from "./palettes.json"
import type { UiStyle } from "./ui-style"
import { nativeApp } from "./platform"

export type Palette = keyof typeof palettes
export type AppearanceMode = "system" | "light" | "dark"
export const PALETTES = palettes
const paletteKey = "xique-mobile-palette-v1", modeKey = "xique-mobile-theme-mode-v1"
const valid = (v: string | undefined | null): v is Palette => !!v && Object.hasOwn(palettes, v)
function palette(): Palette { const v = document.documentElement.dataset.mobilePalette; return valid(v) ? v : "original" }
function mode(): AppearanceMode { const v = document.documentElement.dataset.mobileMode; return v === "system" || v === "dark" ? v : "light" }
const observe = (callback: () => void) => { const o = new MutationObserver(callback); o.observe(document.documentElement, { attributes: true, attributeFilter: ["data-mobile-palette", "data-mobile-mode"] }); return () => o.disconnect() }
export function setPalette(v: Palette) { if (!valid(v)) return; document.documentElement.dataset.mobilePalette = v; try { localStorage.setItem(paletteKey, v) } catch { /* Session still updates. */ } }
export function setAppearanceMode(v: AppearanceMode) { if (!["system", "light", "dark"].includes(v)) return; document.documentElement.dataset.mobileMode = v; try { localStorage.setItem(modeKey, v) } catch { /* Session still updates. */ } applyTheme(v === "system" ? matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light" : v) }
const bodyKeys = ["--xq-base", "--xq-bg", "--xq-surface", "--xq-text", "--xq-muted", "--xq-border", "--xq-blue", "--xq-soft", "--background", "--foreground", "--card", "--card-foreground", "--muted", "--muted-foreground", "--border", "--primary", "--primary-foreground", "--pref-primary", "--pref-on-primary", "--pref-nav", "--pref-selected"]
const glassKeys = ["tint", "line", "shadow", "ink", "muted", "nav", "selected", "top", "bottom", "ambient", "lilac", "base"]
function clearTokens() { bodyKeys.forEach(k => document.body.style.removeProperty(k)); glassKeys.forEach(k => document.documentElement.style.removeProperty("--xm-glass-" + k)) }
function tokens(id: Palette, theme: "light" | "dark", style: UiStyle) {
  clearTokens(); if (id === "original") {
    if (style === "android") {
      const dark = theme === "dark", bg = dark ? "#17171e" : "#faf9ff", well = dark ? "#24242e" : "#f0eff7", surface = dark ? "#202029" : "#f5f4fa", text = dark ? "#e6e3f0" : "#202027", muted = dark ? "#b6b4c4" : "#626172", line = dark ? "#474651" : "#d1cedd", primary = dark ? "#bdc4ff" : "#535d94", on = dark ? "#252e60" : "#ffffff", selected = dark ? "#373e64" : "#e0e3ff";
      const values = [bg, well, surface, text, muted, line, primary, selected, bg, text, surface, text, well, muted, line, primary, on, primary, on, surface, selected];
      bodyKeys.forEach((k, i) => document.body.style.setProperty(k, values[i]));
    }
    return
  }
  const p = palettes[id][theme], g = p.glass, body = document.body.style, root = document.documentElement.style
  const values = [p.base, p.well, p.surface, p.text, p.muted, p.border, p.primary, p.well, p.base, p.text, p.surface, p.text, p.well, p.muted, p.border, p.primary, p.onPrimary, p.primary, p.onPrimary, p.nav, p.selection]
  bodyKeys.forEach((k, i) => body.setProperty(k, values[i]))
  const glass: Record<string, string> = { ...g, tint: style === "glass" ? g.tint.slice(0, 7) + (theme === "light" ? "b0" : "b8") : g.tint, selected: g.selection, ink: p.text, muted: p.muted, shadow: theme === "light" ? "0 8px 28px #43586f12" : "0 8px 28px #0000002b" }
  glassKeys.forEach(k => root.setProperty("--xm-glass-" + k, glass[k]))
}
export function useMobilePresentation(style: UiStyle) {
  const { theme } = useTheme(), selected = useSyncExternalStore(observe, palette), selectedMode = useSyncExternalStore(observe, mode)
  useLayoutEffect(() => tokens(selected, theme, style), [selected, theme, style])
  useEffect(() => () => clearTokens(), [])
  useEffect(() => { if (selectedMode !== "system") return; const query = matchMedia("(prefers-color-scheme: dark)"), changed = () => applyTheme(query.matches ? "dark" : "light"); changed(); query.addEventListener("change", changed); return () => query.removeEventListener("change", changed) }, [selectedMode])
  return { theme, palette: selected, mode: selectedMode, setPalette, setMode: setAppearanceMode }
}
if (nativeApp) try { setPalette(valid(localStorage.getItem(paletteKey)) ? localStorage.getItem(paletteKey) as Palette : "original"); const m = localStorage.getItem(modeKey); setAppearanceMode(m === "system" || m === "dark" || m === "light" ? m : document.documentElement.classList.contains("dark") ? "dark" : "light") } catch { document.documentElement.dataset.mobilePalette = "original" }
