import { useCallback, useSyncExternalStore } from "react"

const THEME_KEY = "kairos-theme"
const ACCENT_KEY = "kairos-accent"

export type AccentColor = "blue" | "cyan" | "emerald" | "violet" | "rose" | "amber"

export interface AccentOption {
  value: AccentColor
  label: string
  swatchClass: string
  swatchColor: string
}

export const ACCENT_OPTIONS: AccentOption[] = [
  { value: "blue", label: "清蓝", swatchClass: "bg-sky-500", swatchColor: "#0ea5e9" },
  { value: "cyan", label: "湖青", swatchClass: "bg-cyan-500", swatchColor: "#06b6d4" },
  { value: "emerald", label: "松绿", swatchClass: "bg-emerald-500", swatchColor: "#10b981" },
  { value: "violet", label: "藤紫", swatchClass: "bg-violet-500", swatchColor: "#8b5cf6" },
  { value: "rose", label: "蔷薇", swatchClass: "bg-rose-500", swatchColor: "#f43f5e" },
  { value: "amber", label: "琥珀", swatchClass: "bg-amber-500", swatchColor: "#f59e0b" },
]

function isAccentColor(value: string | null): value is AccentColor {
  return ACCENT_OPTIONS.some((option) => option.value === value)
}

function getSnapshot(): "dark" | "light" {
  return document.documentElement.classList.contains("dark") ? "dark" : "light"
}

function getAccentSnapshot(): AccentColor {
  const accent = document.documentElement.dataset.accent ?? null
  return isAccentColor(accent) ? accent : "blue"
}

function subscribe(callback: () => void) {
  const observer = new MutationObserver(callback)
  observer.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ["class", "data-accent"],
  })
  return () => observer.disconnect()
}

function resolveTheme(): "dark" | "light" {
  const stored = localStorage.getItem(THEME_KEY)
  if (stored === "dark" || stored === "light") return stored
  return window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light"
}

function resolveAccentColor(): AccentColor {
  const stored = localStorage.getItem(ACCENT_KEY)
  return isAccentColor(stored) ? stored : "blue"
}

export function applyTheme(theme: "dark" | "light") {
  const root = document.documentElement
  if (theme === "dark") {
    root.classList.add("dark")
  } else {
    root.classList.remove("dark")
  }
  localStorage.setItem(THEME_KEY, theme)
  // 触发自定义事件通知 FluentProvider
  window.dispatchEvent(new Event('theme-change'))
}

export function applyAccentColor(accent: AccentColor) {
  document.documentElement.dataset.accent = accent
  localStorage.setItem(ACCENT_KEY, accent)
}

export function useTheme() {
  const theme = useSyncExternalStore(subscribe, getSnapshot)
  const accent = useSyncExternalStore(subscribe, getAccentSnapshot)

  const toggle = useCallback(() => {
    applyTheme(theme === "dark" ? "light" : "dark")
  }, [theme])

  const setTheme = useCallback((t: "dark" | "light") => {
    applyTheme(t)
  }, [])

  const setAccent = useCallback((nextAccent: AccentColor) => {
    applyAccentColor(nextAccent)
  }, [])

  return { theme, accent, toggle, setTheme, setAccent }
}

applyTheme(resolveTheme())
applyAccentColor(resolveAccentColor())
