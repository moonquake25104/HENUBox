import type { Course } from "@/types/course"

/** Light, clean pastel surfaces with dark text in either appearance mode. */
export const COURSE_COLORS = [
  { value: "#D7EBDC", label: "薄荷绿" },
  { value: "#D4E6FA", label: "雾蓝" },
  { value: "#E8DDF6", label: "藕紫" },
  { value: "#FFF0CC", label: "奶油黄" },
  { value: "#F8DCE2", label: "樱粉" },
  { value: "#D3EEF0", label: "浅青" },
  { value: "#F9E2D1", label: "杏橙" },
  { value: "#E1E8F3", label: "月光蓝" },
] as const
const legacy = ["#86C0AD", "#9DB8D9", "#D3B5D8", "#C7B48A", "#D8B9C7", "#B8C9A8", "#E6B49A", "#A9C6E2"]

const DEFAULT_COLOR = "#7C8CC0"

/** Existing custom colors win; old uniform defaults get a stable per-course color. */
export function courseDisplayColor(course: Pick<Course, "id" | "color"> & Partial<Pick<Course, "name">>) {
  const color = typeof course.color === "string" ? course.color.trim().toUpperCase() : ""
  const old = legacy.indexOf(color)
  if (old >= 0) return COURSE_COLORS[old].value
  if (color && color !== DEFAULT_COLOR && /^#[0-9A-F]{6}$/.test(color)) return color
  let hash = 2166136261
  const name = course.name?.trim() || String(course.id)
  for (const char of name) hash = Math.imul(hash ^ char.charCodeAt(0), 16777619) >>> 0
  return COURSE_COLORS[hash % COURSE_COLORS.length].value
}
