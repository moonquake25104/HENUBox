import type { Course } from "@/types/course"

export const DAY_LABELS = ["周一", "周二", "周三", "周四", "周五", "周六", "周日"]
export const DAY_SHORT = ["一", "二", "三", "四", "五", "六", "日"]
export const HOUR_HEIGHT = 44
export const START_HOUR = 7
export const END_HOUR = 22
export const TOTAL_HOURS = END_HOUR - START_HOUR

export const FIELD_CLASS =
  "w-full h-9 rounded-md border border-input bg-background px-3 py-1 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-ring focus:border-ring"

export const COLOR_OPTIONS = [
  { value: "#B8C9E8", label: "雾蓝" },
  { value: "#C5E0D8", label: "薄荷" },
  { value: "#F0D5D8", label: "豆沙粉" },
  { value: "#E8D5F0", label: "香芋紫" },
  { value: "#F5E6C8", label: "鹅黄" },
  { value: "#D5E8F0", label: "浅海蓝" },
  { value: "#F0E0D0", label: "暖杏" },
  { value: "#D8E8D0", label: "嫩绿" },
]

export interface CourseFormData {
  name: string
  day_of_week: number
  start_time: string
  end_time: string
  week_pattern: string
  semester_start_date: string
  location: string
  teacher: string
  color: string
  semester: string
}

export function emptyForm(): CourseFormData {
  return {
    name: "",
    day_of_week: 1,
    start_time: "08:30",
    end_time: "10:10",
    week_pattern: "",
    semester_start_date: "",
    location: "",
    teacher: "",
    color: "#B8C9E8",
    semester: "",
  }
}

export function courseToForm(c: Course): CourseFormData {
  return {
    name: c.name,
    day_of_week: c.day_of_week,
    start_time: c.start_time,
    end_time: c.end_time,
    week_pattern: c.week_pattern,
    semester_start_date: c.semester_start_date,
    location: c.location,
    teacher: c.teacher,
    color: c.color,
    semester: c.semester,
  }
}

export function timeToMinutes(time: string): number {
  const [h, m] = time.split(":").map(Number)
  return h * 60 + m
}

export function minutesToTop(minutes: number): number {
  return ((minutes - START_HOUR * 60) / 60) * HOUR_HEIGHT
}

function hexToRgb(hex: string): { r: number; g: number; b: number } | null {
  const match = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex)
  if (!match) return null
  return {
    r: parseInt(match[1], 16),
    g: parseInt(match[2], 16),
    b: parseInt(match[3], 16),
  }
}

export function isDarkColor(hex: string): boolean {
  const rgb = hexToRgb(hex)
  if (!rgb) return false
  const luminance = (0.299 * rgb.r + 0.587 * rgb.g + 0.114 * rgb.b) / 255
  return luminance <= 0.55
}

export function hexToRgba(hex: string, alpha: number): string {
  const rgb = hexToRgb(hex)
  if (!rgb) return hex
  return `rgba(${rgb.r}, ${rgb.g}, ${rgb.b}, ${alpha})`
}

export function todayKey(): string {
  const d = new Date()
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

export function computeCurrentWeek(startDate: string): number {
  const start = new Date(`${startDate}T00:00:00`)
  if (Number.isNaN(start.getTime())) return 1
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  const diffDays = Math.floor((today.getTime() - start.getTime()) / 86_400_000)
  return Math.max(1, Math.floor(diffDays / 7) + 1)
}

export function dayCellKey(weekStartDate: string, offset: number): string {
  const start = new Date(`${weekStartDate}T00:00:00`)
  if (Number.isNaN(start.getTime())) return ""
  start.setDate(start.getDate() + offset)
  const month = String(start.getMonth() + 1).padStart(2, "0")
  const day = String(start.getDate()).padStart(2, "0")
  return `${start.getFullYear()}-${month}-${day}`
}
