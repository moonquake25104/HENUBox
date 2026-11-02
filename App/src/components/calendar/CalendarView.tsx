import type { CSSProperties } from "react"
import { useEffect, useState } from "react"
import type {
  CalendarEvent,
  CalendarWeekCmd,
  CalendarWeekResponse,
} from "@/types/schedule"
import { getCalendarWeek } from "@/lib/api/calendar"
import { cn } from "@/lib/utils"
import {
  BookOpen,
  CalendarDays,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  GraduationCap,
  ListTodo,
  MapPin,
  Repeat,
} from "lucide-react"

const DAY_SHORT = ["一", "二", "三", "四", "五", "六", "日"]
const HOUR_HEIGHT = 44
const START_HOUR = 7
const END_HOUR = 22
const TOTAL_HOURS = END_HOUR - START_HOUR
const COMPACT_TIME_AXIS_WIDTH = 28

type CalendarMode = "month" | "week" | "day"

function hexToRgb(hex: string): { r: number; g: number; b: number } | null {
  const match = /^#?([a-f\d]{2})([a-f\d]{2})([a-f\d]{2})$/i.exec(hex)
  if (!match) return null
  return {
    r: parseInt(match[1], 16),
    g: parseInt(match[2], 16),
    b: parseInt(match[3], 16),
  }
}

function hexToRgba(hex: string, alpha: number): string {
  const rgb = hexToRgb(hex)
  if (!rgb) return hex
  return `rgba(${rgb.r}, ${rgb.g}, ${rgb.b}, ${alpha})`
}

function isDarkColor(hex: string): boolean {
  const rgb = hexToRgb(hex)
  if (!rgb) return false
  const luminance = (0.299 * rgb.r + 0.587 * rgb.g + 0.114 * rgb.b) / 255
  return luminance <= 0.55
}

function timeToMinutes(time: string): number {
  const [h, m] = time.split(":").map(Number)
  if (!Number.isFinite(h) || !Number.isFinite(m)) return START_HOUR * 60
  return h * 60 + m
}

function clampMinutes(minutes: number): number {
  return Math.max(START_HOUR * 60, Math.min(END_HOUR * 60, minutes))
}

function minutesToTop(minutes: number): number {
  return ((clampMinutes(minutes) - START_HOUR * 60) / 60) * HOUR_HEIGHT
}

function todayKey(): string {
  const d = new Date()
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

function dayCellKey(weekStartDate: string, offset: number): string {
  const start = new Date(`${weekStartDate}T00:00:00`)
  if (Number.isNaN(start.getTime())) return ""
  start.setDate(start.getDate() + offset)
  const month = String(start.getMonth() + 1).padStart(2, "0")
  const day = String(start.getDate()).padStart(2, "0")
  return `${start.getFullYear()}-${month}-${day}`
}

function startOfWeekKey(date = new Date()): string {
  const d = new Date(date)
  d.setHours(0, 0, 0, 0)
  const dayFromMonday = d.getDay() === 0 ? 6 : d.getDay() - 1
  d.setDate(d.getDate() - dayFromMonday)
  const year = d.getFullYear()
  const month = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${year}-${month}-${day}`
}

function addDaysToKey(dateKey: string, days: number): string {
  const date = new Date(`${dateKey}T00:00:00`)
  if (Number.isNaN(date.getTime())) return startOfWeekKey()
  date.setDate(date.getDate() + days)
  return dayCellKey(startOfWeekKey(date), 0)
}

function formatWeekRange(startDate: string, endDate: string): string {
  if (!startDate || !endDate) return "日期周"
  return `${startDate.slice(5)} - ${endDate.slice(5)}`
}

function formatMonthLabel(startDate: string, endDate: string): string {
  const start = new Date(`${startDate}T00:00:00`)
  const end = new Date(`${endDate}T00:00:00`)
  if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) return "日历"

  const startYear = start.getFullYear()
  const endYear = end.getFullYear()
  const startMonth = start.getMonth() + 1
  const endMonth = end.getMonth() + 1

  if (startYear === endYear && startMonth === endMonth) {
    return `${startYear}年${startMonth}月`
  }
  if (startYear === endYear) {
    return `${startYear}年${startMonth}月 / ${endMonth}月`
  }
  return `${startYear}年${startMonth}月 / ${endYear}年${endMonth}月`
}

function toDateKey(year: number, month: number, day: number): string {
  const m = String(month).padStart(2, "0")
  const d = String(day).padStart(2, "0")
  return `${year}-${m}-${d}`
}

/** 生成月视图所需的 35/42 个日期单元（周一开始）。 */
function getMonthGridDays(
  year: number,
  month: number,
): Array<{ dateKey: string; day: number; inCurrentMonth: boolean }> {
  const firstDay = new Date(year, month - 1, 1)
  const lastDay = new Date(year, month, 0)
  const dayOfWeek = firstDay.getDay() === 0 ? 6 : firstDay.getDay() - 1
  const gridStart = new Date(firstDay)
  gridStart.setDate(gridStart.getDate() - dayOfWeek)

  const lastDayOfWeek = lastDay.getDay() === 0 ? 6 : lastDay.getDay() - 1
  const gridEnd = new Date(lastDay)
  gridEnd.setDate(gridEnd.getDate() + (6 - lastDayOfWeek))

  const totalCells = Math.round((gridEnd.getTime() - gridStart.getTime()) / 86400000) + 1
  const days: Array<{ dateKey: string; day: number; inCurrentMonth: boolean }> = []

  for (let i = 0; i < totalCells; i++) {
    const d = new Date(gridStart)
    d.setDate(gridStart.getDate() + i)
    days.push({
      dateKey: toDateKey(d.getFullYear(), d.getMonth() + 1, d.getDate()),
      day: d.getDate(),
      inCurrentMonth: d.getMonth() + 1 === month,
    })
  }
  return days
}

/** 获取覆盖某月所需的各周起始日期（YYYY-MM-DD）。 */
function getMonthWeekStarts(year: number, month: number): string[] {
  const firstDay = new Date(year, month - 1, 1)
  const dayOfWeek = firstDay.getDay() === 0 ? 6 : firstDay.getDay() - 1
  const gridStart = new Date(firstDay)
  gridStart.setDate(gridStart.getDate() - dayOfWeek)

  const lastDay = new Date(year, month, 0)
  const lastDayOfWeek = lastDay.getDay() === 0 ? 6 : lastDay.getDay() - 1
  const gridEnd = new Date(lastDay)
  gridEnd.setDate(gridEnd.getDate() + (6 - lastDayOfWeek))

  const weeks: string[] = []
  const cursor = new Date(gridStart)
  while (cursor <= gridEnd) {
    weeks.push(toDateKey(cursor.getFullYear(), cursor.getMonth() + 1, cursor.getDate()))
    cursor.setDate(cursor.getDate() + 7)
  }
  return weeks
}

/** 将多周事件数据按日期键（YYYY-MM-DD）归并。 */
type EventsByDate = Record<string, CalendarEvent[]>

function buildEventsByDate(weeksData: CalendarWeekResponse[]): EventsByDate {
  const byDate: EventsByDate = {}
  for (const week of weeksData) {
    for (const event of week.events) {
      const base = new Date(`${week.week_start_date}T00:00:00`)
      base.setDate(base.getDate() + (event.day_of_week - 1))
      const key = toDateKey(base.getFullYear(), base.getMonth() + 1, base.getDate())
      if (!byDate[key]) byDate[key] = []
      byDate[key].push(event)
    }
  }
  return byDate
}

/** 解析 YYYY-MM-DD 日期键中的月份，返回 YYYY-MM-01。 */
function monthKeyFromDateKey(dateKey: string): string {
  return dateKey.length >= 7 ? `${dateKey.slice(0, 7)}-01` : dateKey
}

/** 对 YYYY-MM-01 月份键加减月数。 */
function addMonthsToKey(monthKey: string, delta: number): string {
  const d = new Date(`${monthKey}T00:00:00`)
  if (Number.isNaN(d.getTime())) return monthKey
  d.setMonth(d.getMonth() + delta)
  return toDateKey(d.getFullYear(), d.getMonth() + 1, 1)
}

function eventTypeLabel(event: CalendarEvent): string {
  switch (event.kind) {
    case "course":
      return "课程"
    case "exam":
      return "考试"
    case "task":
      return event.tags.includes("每日") ? "每日" : "待办"
  }
}

function eventTimeLabel(event: CalendarEvent): string {
  if (event.kind === "task" && event.tags.includes("每日")) return "每日"
  if (event.kind === "task") return "全天截止"
  return `${event.start_time}-${event.end_time}`
}

function eventToneClass(kind: CalendarEvent["kind"]): string {
  switch (kind) {
    case "course":
      return "text-primary"
    case "exam":
      return "text-destructive"
    case "task":
      return "text-amber-600 dark:text-amber-300"
  }
}

function EventTypeIcon({
  kind,
  className,
}: {
  kind: CalendarEvent["kind"]
  className?: string
}) {
  if (kind === "course") return <BookOpen className={className} />
  if (kind === "exam") return <GraduationCap className={className} />
  return <ListTodo className={className} />
}

interface EventCardProps {
  event: CalendarEvent
  compact?: boolean
  onClick: () => void
  className?: string
}

function EventCard({ event, compact = false, onClick, className }: EventCardProps) {
  const isDone = event.kind === "task" && event.tags.includes("完成")
  const isExam = event.kind === "exam"
  const markerStyle: CSSProperties = {
    backgroundColor: isDone ? undefined : event.color,
  }
  const cardStyle: CSSProperties = {
    borderColor: isExam ? event.color : undefined,
    backgroundColor: isExam ? hexToRgba(event.color, 0.08) : undefined,
  }

  return (
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex w-full items-start gap-3 rounded-lg border border-border/60 bg-card/75 text-left transition-colors hover:bg-card active:bg-muted/60",
        compact ? "min-h-14 px-2.5 py-2" : "min-h-16 px-3 py-3",
        isExam && "border-dashed",
        className,
      )}
      style={cardStyle}
    >
      <span
        className={cn(
          "mt-0.5 flex shrink-0 items-center justify-center rounded-lg text-white",
          compact ? "h-8 w-8" : "h-9 w-9",
          isDone && "bg-muted text-muted-foreground",
        )}
        style={markerStyle}
      >
        {isDone ? (
          <CheckCircle2 className="h-4 w-4" />
        ) : (
          <EventTypeIcon kind={event.kind} className="h-4 w-4" />
        )}
      </span>

      <span className="min-w-0 flex-1">
        <span className="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-xs text-muted-foreground">
          <span className={cn("font-medium", eventToneClass(event.kind))}>
            {eventTypeLabel(event)}
          </span>
          <span className="tabular-nums">{eventTimeLabel(event)}</span>
        </span>
        <span
          className={cn(
            "mt-1 block break-words font-semibold leading-snug text-foreground",
            compact ? "text-xs" : "text-sm",
            isDone && "text-muted-foreground line-through",
          )}
        >
          {event.title}
        </span>
        {event.location && (
          <span className="mt-1 flex items-center gap-1 text-xs text-muted-foreground">
            <MapPin className="h-3 w-3 shrink-0" />
            <span className="truncate">{event.location}</span>
          </span>
        )}
        {!compact && event.tags.length > 0 && (
          <span className="mt-2 flex flex-wrap gap-1">
            {event.tags.slice(0, 3).map((tag) => (
              <span
                key={`${event.kind}-${event.id}-${tag}`}
                className="rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground"
              >
                {tag}
              </span>
            ))}
          </span>
        )}
      </span>
    </button>
  )
}

function getEventBlockMetrics(event: CalendarEvent, minHeight: number) {
  const startMin = timeToMinutes(event.start_time)
  const endMin = Math.max(startMin + 30, timeToMinutes(event.end_time))
  const dayEnd = END_HOUR * 60
  const clampedStart = Math.min(clampMinutes(startMin), dayEnd - 30)
  const clampedEnd = Math.min(dayEnd, Math.max(clampedStart + 30, clampMinutes(endMin)))

  return {
    top: minutesToTop(clampedStart),
    height: Math.max(minHeight, ((clampedEnd - clampedStart) / 60) * HOUR_HEIGHT - 2),
  }
}

interface CalendarWeekTimetableProps {
  weekData: CalendarWeekResponse
  allEvents: CalendarEvent[]
  today: string
  selectedDayIndex: number
  compact?: boolean
  onDayClick: (dayIndex: number) => void
  onEventClick: (event: CalendarEvent) => void
}

function CalendarWeekTimetable({
  weekData,
  allEvents,
  today,
  selectedDayIndex,
  compact = false,
  onDayClick,
  onEventClick,
}: CalendarWeekTimetableProps) {
  const gridTemplateColumns = compact
    ? `${COMPACT_TIME_AXIS_WIDTH}px repeat(7, minmax(0, 1fr))`
    : "48px repeat(7, minmax(0, 1fr))"
  const timelineHeight = TOTAL_HOURS * HOUR_HEIGHT

  return (
    <div className={cn(compact ? "overflow-x-hidden pb-4" : "overflow-x-auto px-1 pb-4")}>
      <div
        className={cn(
          "grid w-full select-none",
          compact ? "min-w-0" : "min-w-[680px]",
        )}
      >
        <div className="grid" style={{ gridTemplateColumns }}>
          <div />
          {DAY_SHORT.map((day, dayIndex) => {
            const dateKey = dayCellKey(weekData.week_start_date, dayIndex)
            const dateNum = dateKey ? dateKey.slice(8) : ""
            const isToday = dateKey === today

            return (
              <button
                key={day}
                type="button"
                onClick={() => onDayClick(dayIndex)}
                className={cn(
                  "flex h-12 min-w-0 flex-col items-center justify-center text-center transition-colors",
                  isToday
                    ? "bg-primary text-primary-foreground shadow-sm"
                    : dayIndex === selectedDayIndex
                      ? "bg-primary/10 text-primary"
                      : "text-foreground hover:bg-muted/40",
                )}
              >
                <span className="text-[10px] font-medium leading-tight">{day}</span>
                <span className="mt-0.5 text-xs font-semibold leading-tight tabular-nums">
                  {dateNum || "--"}
                </span>
              </button>
            )
          })}

          <div className={cn(
            "border-y border-border/35 bg-card text-[10px] font-medium text-muted-foreground",
            compact ? "px-1 py-2 text-center" : "px-1.5 py-2",
          )}>
            {compact ? "每" : "每日"}
          </div>
          {DAY_SHORT.map((day, dayIndex) => {
            const dateKey = dayCellKey(weekData.week_start_date, dayIndex)
            const dailyEvents = allEvents.filter(
              (event) => event.kind === "task" && event.tags.includes("每日") && event.day_of_week === dayIndex + 1,
            )
            const doneToday = dailyEvents.filter((e) => e.tags.includes("完成")).length
            const totalToday = dailyEvents.length
            const isToday = dateKey === today

            return (
              <div
                key={`daily-${day}`}
                className={cn(
                  "min-h-12 border-y border-l border-border/35 px-1 py-1.5",
                  isToday && "bg-primary/[0.04]",
                )}
              >
                {totalToday === 0 ? (
                  <span className="block px-1 py-1 text-[10px] text-muted-foreground/30">
                    —
                  </span>
                ) : (
                  <>
                    {isToday && totalToday > 0 && (
                      <span className="mb-1 inline-flex items-center rounded-full bg-emerald-500/15 px-1.5 py-0.5 text-[9px] font-semibold text-emerald-500 tabular-nums">
                        {doneToday}/{totalToday}
                      </span>
                    )}
                    <div className="space-y-1">
                      {dailyEvents.map((event) => {
                        const isDone = event.tags.includes("完成")
                        const dailyStyle: CSSProperties = {
                          borderColor: event.color,
                          backgroundColor: hexToRgba(event.color, 0.10),
                        }

                        return (
                          <button
                            key={`daily-${event.id}-${event.day_of_week}`}
                            type="button"
                            onClick={() => onEventClick(event)}
                            title={`${event.title} · 每日${isDone ? " · 已完成" : " · 待完成"}`}
                            className={cn(
                              "flex min-h-7 w-full items-center gap-1 rounded-md border px-1.5 py-1 text-left text-[10px] font-medium leading-tight transition-colors hover:bg-card",
                              isDone && "text-muted-foreground line-through",
                            )}
                            style={dailyStyle}
                          >
                            {isDone ? (
                              <CheckCircle2 className="h-3 w-3 shrink-0 text-emerald-400" />
                            ) : (
                              <Repeat className="h-3 w-3 shrink-0 text-amber-400" />
                            )}
                            <span className="line-clamp-2">{event.title}</span>
                          </button>
                        )
                      })}
                    </div>
                  </>
                )}
              </div>
            )
          })}

          <div className={cn(
            "border-y border-border/35 bg-card text-[10px] font-medium text-muted-foreground",
            compact ? "px-1 py-2 text-center" : "px-1.5 py-2",
          )}>
            {compact ? "截" : "截止"}
          </div>
          {DAY_SHORT.map((day, dayIndex) => {
            const dateKey = dayCellKey(weekData.week_start_date, dayIndex)
            const taskEvents = allEvents.filter(
              (event) => event.kind === "task" && !event.tags.includes("每日") && event.day_of_week === dayIndex + 1,
            )
            const isToday = dateKey === today

            return (
              <div
                key={`tasks-${day}`}
                className={cn(
                  "min-h-12 border-y border-l border-border/35 px-1 py-1.5",
                  isToday && "bg-primary/[0.04]",
                )}
              >
                {taskEvents.length === 0 ? (
                  <span className="block px-1 py-1 text-[10px] text-muted-foreground/50">
                    无
                  </span>
                ) : (
                  <div className="space-y-1">
                    {taskEvents.map((event) => {
                      const isDone = event.tags.includes("完成")
                      const taskStyle: CSSProperties = {
                        borderColor: event.color,
                        backgroundColor: hexToRgba(event.color, 0.12),
                      }

                      return (
                        <button
                          key={`task-${event.id}`}
                          type="button"
                          onClick={() => onEventClick(event)}
                          title={`${event.title} · ${eventTimeLabel(event)}${isDone ? " · 已完成" : ""}`}
                          className={cn(
                            "flex min-h-7 w-full items-center gap-1 rounded-md border px-1.5 py-1 text-left text-[10px] font-medium leading-tight transition-colors hover:bg-card",
                            isDone && "text-muted-foreground line-through",
                          )}
                          style={taskStyle}
                        >
                          <ListTodo className="h-3 w-3 shrink-0" />
                          <span className="line-clamp-2">{event.title}</span>
                        </button>
                      )
                    })}
                  </div>
                )}
              </div>
            )
          })}

          <div className="relative bg-card" style={{ height: `${timelineHeight}px` }}>
            {Array.from({ length: TOTAL_HOURS }, (_, i) => {
              if (i === 0) return <div key={i} style={{ height: `${HOUR_HEIGHT}px` }} />
              return (
                <div
                  key={i}
                  className={cn(
                    "absolute text-muted-foreground/70 tabular-nums",
                    compact ? "right-1 text-[9px]" : "right-1.5 text-[10px]",
                  )}
                  style={{ top: `${i * HOUR_HEIGHT - (compact ? 5 : 6)}px` }}
                >
                  {compact
                    ? String(START_HOUR + i).padStart(2, "0")
                    : `${String(START_HOUR + i).padStart(2, "0")}:00`}
                </div>
              )
            })}
          </div>

          {DAY_SHORT.map((day, dayIndex) => {
            const dateKey = dayCellKey(weekData.week_start_date, dayIndex)
            const isToday = dateKey === today
            const isSelected = dayIndex === selectedDayIndex
            const timedEvents = allEvents.filter(
              (event) => event.kind !== "task" && event.day_of_week === dayIndex + 1,
            )

            return (
              <div
                key={`timeline-${day}`}
                className={cn(
                  "relative border-l border-border/25",
                  isToday && "bg-primary/[0.04]",
                  isSelected && !isToday && "bg-primary/[0.02]",
                )}
                style={{ height: `${timelineHeight}px` }}
              >
                {Array.from({ length: TOTAL_HOURS }, (_, i) => (
                  <div
                    key={i}
                    className="absolute inset-x-0 border-t border-border/25"
                    style={{ top: `${i * HOUR_HEIGHT}px` }}
                  />
                ))}

                {timedEvents.map((event) => {
                  const { top, height } = getEventBlockMetrics(event, compact ? 26 : 28)
                  const isExam = event.kind === "exam"
                  const dark = isDarkColor(event.color)
                  const blockStyle: CSSProperties = {
                    top: `${top}px`,
                    height: `${height}px`,
                    backgroundColor: isExam ? hexToRgba(event.color, 0.16) : event.color,
                    borderColor: isExam ? event.color : undefined,
                  }

                  return (
                    <button
                      key={`${event.kind}-${event.id}`}
                      type="button"
                      onClick={() => onEventClick(event)}
                      title={`${event.title} — ${event.start_time}${event.location ? ` — ${event.location}` : ""}`}
                      className={cn(
                        "absolute overflow-hidden rounded-lg text-left transition-all active:brightness-90",
                        compact ? "left-px right-px px-1.5 py-1" : "left-0.5 right-0.5 px-2 py-1.5",
                        isExam ? "border-2 border-dashed" : "hover:z-20 hover:brightness-105 hover:shadow-lg",
                        !isExam && dark ? "text-white" : "text-foreground",
                      )}
                      style={blockStyle}
                    >
                      {compact ? (
                        <span className="flex h-full min-w-0 flex-col justify-center">
                          <span className="line-clamp-2 text-[10px] font-semibold leading-tight">
                            {event.title}
                          </span>
                          {height >= 36 && (
                            <span className="block truncate text-[9px] leading-tight opacity-70">
                              {event.start_time}
                            </span>
                          )}
                        </span>
                      ) : (
                        <>
                          <span className="flex items-center gap-1">
                            <EventTypeIcon kind={event.kind} className="h-3.5 w-3.5 shrink-0" />
                            <span className="line-clamp-2 text-[11px] font-semibold leading-tight">
                              {event.title}
                            </span>
                          </span>
                          <span className="block text-[9px] leading-tight opacity-70">
                            {event.start_time}
                          </span>
                          {event.location && (
                            <span className="block truncate text-[10px] leading-tight opacity-70">
                              {event.location}
                            </span>
                          )}
                        </>
                      )}
                    </button>
                  )
                })}
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}

interface CalendarMonthGridProps {
  gridDays: Array<{ dateKey: string; day: number; inCurrentMonth: boolean }>
  eventsByDate: EventsByDate
  today: string
  selectedDateKey: string
  onDateClick: (dateKey: string) => void
}

/** 月视图网格：展示日期数字、事件摘要，点击日期切换到日视图。 */
function CalendarMonthGrid({
  gridDays,
  eventsByDate,
  today,
  selectedDateKey,
  onDateClick,
}: CalendarMonthGridProps) {
  const rows: Array<typeof gridDays> = []
  for (let i = 0; i < gridDays.length; i += 7) {
    rows.push(gridDays.slice(i, i + 7))
  }

  return (
    <div className="mx-auto w-full max-w-4xl px-2 pb-4">
      {/* 星期头 */}
      <div className="grid grid-cols-7 mb-1">
        {DAY_SHORT.map((day) => (
          <div
            key={day}
            className="py-1.5 text-center text-[10px] font-medium text-muted-foreground/60"
          >
            {day}
          </div>
        ))}
      </div>
      {/* 日期网格 */}
      <div className="rounded-xl border border-border/30 overflow-hidden">
        {rows.map((weekDays, rowIdx) => (
          <div key={rowIdx} className="grid grid-cols-7">
            {weekDays.map((cell) => {
              const cellEvents = eventsByDate[cell.dateKey] ?? []
              const isToday = cell.dateKey === today
              const isSelected = cell.dateKey === selectedDateKey

              // 按来源分组显示摘要；每日任务独立为状态圆点
              const courseEvents = cellEvents.filter((e) => e.kind === "course")
              const examEvents = cellEvents.filter((e) => e.kind === "exam")
              const dailyEvents = cellEvents.filter((e) => e.kind === "task" && e.tags.includes("每日"))
              const taskEvents = cellEvents.filter((e) => e.kind === "task" && !e.tags.includes("每日"))
              const summaryEvents = [...courseEvents, ...examEvents, ...taskEvents]
              const hasSummary = summaryEvents.length > 0
              const hasEvents = hasSummary || dailyEvents.length > 0

              return (
                <button
                  key={cell.dateKey}
                  type="button"
                  onClick={() => onDateClick(cell.dateKey)}
                  className={cn(
                    "flex min-h-[72px] flex-col items-stretch border-b border-r border-border/25 p-1 text-left transition-colors md:min-h-[88px]",
                    "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-ring",
                    !cell.inCurrentMonth && "bg-muted/20",
                    isToday && "bg-primary/[0.04]",
                    isSelected && "ring-2 ring-inset ring-primary",
                  )}
                  aria-label={`${cell.dateKey}，${summaryEvents.length} 个安排`}
                >
                  {/* 日期数字 */}
                  <span
                    className={cn(
                      "mb-0.5 inline-flex h-6 w-6 items-center justify-center rounded-full text-xs font-semibold tabular-nums shrink-0",
                      !cell.inCurrentMonth && "text-muted-foreground/40",
                      cell.inCurrentMonth && !isToday && "text-foreground",
                      isToday && "bg-primary text-primary-foreground",
                    )}
                  >
                    {cell.day}
                  </span>
                  {/* 事件摘要 */}
                  <div className="flex min-h-0 flex-1 flex-col gap-px overflow-hidden">
                    {/* 课程：最多显示 1 条 */}
                    {courseEvents.length > 0 && (
                      <span className="line-clamp-1 rounded-sm bg-primary/10 px-1 py-0.5 text-[9px] font-medium text-primary leading-tight">
                        {courseEvents.length === 1
                          ? courseEvents[0].title
                          : `${courseEvents[0].title} +${courseEvents.length - 1}`}
                      </span>
                    )}
                    {/* 考试：最多显示 1 条 */}
                    {examEvents.length > 0 && (
                      <span className="line-clamp-1 rounded-sm bg-destructive/10 px-1 py-0.5 text-[9px] font-medium text-destructive leading-tight">
                        {examEvents.length === 1
                          ? examEvents[0].title
                          : `${examEvents[0].title} +${examEvents.length - 1}`}
                      </span>
                    )}
                    {/* 待办（非每日）：最多显示 1 条 */}
                    {taskEvents.length > 0 && (
                      <span className="line-clamp-1 rounded-sm bg-amber-500/10 px-1 py-0.5 text-[9px] font-medium text-amber-600 dark:text-amber-400 leading-tight">
                        {taskEvents.length === 1
                          ? taskEvents[0].title
                          : `${taskEvents[0].title} +${taskEvents.length - 1}`}
                      </span>
                    )}
                    {/* 每日任务状态圆点 */}
                    {dailyEvents.length > 0 && (
                      <div className="flex flex-wrap items-center gap-px mt-0.5">
                        {dailyEvents.slice(0, 5).map((e) => (
                          <span
                            key={`daily-dot-${e.id}`}
                            className="inline-block h-1.5 w-1.5 rounded-full"
                            style={{ backgroundColor: e.color }}
                          />
                        ))}
                        {dailyEvents.length > 5 && (
                          <span className="text-[9px] font-medium text-muted-foreground/50 ml-px">
                            +{dailyEvents.length - 5}
                          </span>
                        )}
                      </div>
                    )}
                    {/* 更多计数（移动端隐藏，桌面端显示剩余条数） */}
                    {hasSummary && summaryEvents.length > 3 && (
                      <span className="hidden md:block mt-px text-[9px] font-medium text-muted-foreground/60">
                        +{summaryEvents.length - 3} 更多
                      </span>
                    )}
                    {/* 无事件时显示占位 */}
                    {!hasEvents && (
                      <span className="hidden md:block mt-px text-[9px] text-muted-foreground/30">
                        无安排
                      </span>
                    )}
                  </div>
                </button>
              )
            })}
          </div>
        ))}
      </div>
    </div>
  )
}

interface CalendarViewProps {
  onNavigate: (key: string) => void
}

export function CalendarView({ onNavigate }: CalendarViewProps) {
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [weekData, setWeekData] = useState<CalendarWeekResponse | null>(null)
  const [weekStartDate, setWeekStartDate] = useState(() => startOfWeekKey())
  const [viewMode, setViewMode] = useState<CalendarMode>("day")
  const [selectedDayIndex, setSelectedDayIndex] = useState(
    new Date().getDay() === 0 ? 6 : new Date().getDay() - 1,
  )
  const [weekRefresh, setWeekRefresh] = useState(0)

  // 月视图状态
  const [visibleMonthKey, setVisibleMonthKey] = useState(() => {
    const now = new Date()
    return toDateKey(now.getFullYear(), now.getMonth() + 1, 1)
  })
  const [monthWeeksData, setMonthWeeksData] = useState<CalendarWeekResponse[]>([])
  const [monthLoading, setMonthLoading] = useState(false)
  const [monthError, setMonthError] = useState<string | null>(null)
  const [selectedDateKey, setSelectedDateKey] = useState(todayKey)
  const [monthRefresh, setMonthRefresh] = useState(0)

  const semester = (() => {
    const now = new Date()
    const y = now.getFullYear()
    return now.getMonth() >= 1 && now.getMonth() <= 6 ? `${y}S1` : `${y}S2`
  })()

  useEffect(() => {
    let cancelled = false

    async function loadWeek() {
      setLoading(true)
      setError(null)
      try {
        const cmd: CalendarWeekCmd = {
          semester,
          week_index: 1,
          week_start_date: weekStartDate,
        }
        const res = await getCalendarWeek(cmd)
        if (cancelled) return
        setWeekData(res)
      } catch (e) {
        if (!cancelled) {
          setWeekData(null)
          setError(typeof e === "string" ? e : "无法加载日历数据")
        }
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void loadWeek()
    return () => {
      cancelled = true
    }
  }, [semester, weekStartDate, weekRefresh])

  // 月视图数据加载
  useEffect(() => {
    let cancelled = false

    async function loadMonth() {
      setMonthLoading(true)
      setMonthError(null)
      try {
        const year = Number(visibleMonthKey.slice(0, 4))
        const month = Number(visibleMonthKey.slice(5, 7))
        const weekStarts = getMonthWeekStarts(year, month)
        const results = await Promise.all(
          weekStarts.map((ws) =>
            getCalendarWeek({ semester, week_index: 1, week_start_date: ws } satisfies CalendarWeekCmd),
          ),
        )
        if (!cancelled) {
          setMonthWeeksData(results)
        }
      } catch (e) {
        if (!cancelled) {
          setMonthWeeksData([])
          setMonthError(typeof e === "string" ? e : "无法加载月视图数据")
        }
      } finally {
        if (!cancelled) setMonthLoading(false)
      }
    }

    void loadMonth()
    return () => {
      cancelled = true
    }
  }, [semester, visibleMonthKey, monthRefresh])

  const handlePrevNav = () => {
    if (viewMode === "month") {
      setVisibleMonthKey((key) => addMonthsToKey(key, -1))
    } else {
      setWeekStartDate((date) => addDaysToKey(date, -7))
    }
  }

  const handleNextNav = () => {
    if (viewMode === "month") {
      setVisibleMonthKey((key) => addMonthsToKey(key, 1))
    } else {
      setWeekStartDate((date) => addDaysToKey(date, 7))
    }
  }

  const handleDayClick = (dayIndex: number) => {
    setSelectedDayIndex(dayIndex)
    setViewMode("day")
  }

  const handleEventClick = (event: CalendarEvent) => {
    onNavigate(event.source_link)
  }

  /** 月视图点击日期 → 切换日视图，同步 weekStartDate 和 selectedDayIndex。 */
  const handleMonthDateClick = (dateKey: string) => {
    setSelectedDateKey(dateKey)
    // 将 weekStartDate 同步到所选日期所在周的周一
    const d = new Date(`${dateKey}T00:00:00`)
    if (!Number.isNaN(d.getTime())) {
      const dayFromMonday = d.getDay() === 0 ? 6 : d.getDay() - 1
      d.setDate(d.getDate() - dayFromMonday)
      const newWeekStart = toDateKey(d.getFullYear(), d.getMonth() + 1, d.getDate())
      const dayIndex = dayFromMonday
      setWeekStartDate(newWeekStart)
      setSelectedDayIndex(dayIndex)
    }
    setViewMode("day")
  }

  const today = todayKey()
  const currentWeekStart = startOfWeekKey()
  const isCurrentWeek = weekStartDate === currentWeekStart

  // 月视图派生数据
  const monthGridDays = viewMode === "month"
    ? getMonthGridDays(
        Number(visibleMonthKey.slice(0, 4)),
        Number(visibleMonthKey.slice(5, 7)),
      )
    : []
  const monthEventsByDate = viewMode === "month" ? buildEventsByDate(monthWeeksData) : {}
  const monthEventCount = viewMode === "month"
    ? monthGridDays.reduce(
        (total, day) =>
          day.inCurrentMonth ? total + (monthEventsByDate[day.dateKey]?.length ?? 0) : total,
        0,
      )
    : 0
  const monthTitle = viewMode === "month"
    ? `${visibleMonthKey.slice(0, 4)}年${Number(visibleMonthKey.slice(5, 7))}月`
    : ""
  const isCurrentMonth = viewMode === "month" && monthKeyFromDateKey(todayKey()) === visibleMonthKey

  const weekRangeLabel = formatWeekRange(
    weekData?.week_start_date ?? weekStartDate,
    weekData?.week_end_date ?? dayCellKey(weekStartDate, 6),
  )
  const monthLabel = formatMonthLabel(
    weekData?.week_start_date ?? weekStartDate,
    weekData?.week_end_date ?? dayCellKey(weekStartDate, 6),
  )
  const semesterWeekLabel =
    weekData && weekData.week_index >= 1 ? `第 ${weekData.week_index} 周` : "日期周"

  const allEvents = weekData?.events ?? []
  const selectedDate = weekData ? dayCellKey(weekData.week_start_date, selectedDayIndex) : ""
  const selectedEvents = allEvents.filter((event) => event.day_of_week === selectedDayIndex + 1)

  // 日视图分组：每日习惯 → 普通待办（未完成优先）→ 课程/考试（按开始时间）
  const dayDailyEvents = selectedEvents
    .filter((e) => e.kind === "task" && e.tags.includes("每日"))
    .sort((a, b) => {
      const aDone = a.tags.includes("完成")
      const bDone = b.tags.includes("完成")
      if (aDone !== bDone) return aDone ? 1 : -1
      return a.title.localeCompare(b.title)
    })
  const dayTodoEvents = selectedEvents
    .filter((e) => e.kind === "task" && !e.tags.includes("每日"))
    .sort((a, b) => {
      const aDone = a.tags.includes("完成")
      const bDone = b.tags.includes("完成")
      if (aDone !== bDone) return aDone ? 1 : -1
      return a.title.localeCompare(b.title)
    })
  const dayTimedEvents = selectedEvents
    .filter((e) => e.kind !== "task")
    .sort((a, b) => a.start_time.localeCompare(b.start_time))

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 flex-wrap items-center gap-2 px-3 py-2">
        <button
          type="button"
          onClick={handlePrevNav}
          aria-label={viewMode === "month" ? "上一月" : "上一周"}
          className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
        >
          <ChevronLeft className="h-4 w-4" />
        </button>

        <button
          type="button"
          onClick={() => {
            if (viewMode === "month") {
              const now = new Date()
              const todayMonth = toDateKey(now.getFullYear(), now.getMonth() + 1, 1)
              setVisibleMonthKey(todayMonth)
              setSelectedDateKey(todayKey())
            } else if (!isCurrentWeek) {
              setWeekStartDate(currentWeekStart)
            } else {
              setWeekRefresh((n) => n + 1)
            }
          }}
          className="inline-flex min-h-11 items-center rounded-md px-2.5 text-left transition-colors hover:bg-muted/60"
        >
          {viewMode === "month" ? (
            <span className="text-sm font-semibold text-foreground">{monthTitle}</span>
          ) : (
            <span className="flex flex-col leading-tight">
              <span className="text-sm font-semibold text-foreground">{monthLabel}</span>
              <span className="mt-0.5 text-[11px] font-medium tabular-nums text-muted-foreground">
                {weekRangeLabel}
                {isCurrentWeek ? " · 本周" : ""}
              </span>
            </span>
          )}
        </button>

        <button
          type="button"
          onClick={handleNextNav}
          aria-label={viewMode === "month" ? "下一月" : "下一周"}
          className="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-muted/60 hover:text-foreground"
        >
          <ChevronRight className="h-4 w-4" />
        </button>

        {viewMode !== "month" && !isCurrentWeek && (
          <button
            type="button"
            onClick={() => setWeekStartDate(currentWeekStart)}
            className="h-9 shrink-0 rounded-md px-2 text-[11px] font-medium text-primary transition-colors hover:bg-primary/10"
          >
            今天
          </button>
        )}
        {viewMode === "month" && !isCurrentMonth && (
          <button
            type="button"
            onClick={() => {
              const now = new Date()
              const todayMonth = toDateKey(now.getFullYear(), now.getMonth() + 1, 1)
              setVisibleMonthKey(todayMonth)
              setSelectedDateKey(todayKey())
            }}
            className="h-9 shrink-0 rounded-md px-2 text-[11px] font-medium text-primary transition-colors hover:bg-primary/10"
          >
            今天
          </button>
        )}

        <div className="min-w-full flex-1 sm:min-w-0" />

        <div className="grid h-9 grid-cols-3 rounded-lg bg-muted p-1 text-xs font-medium">
          {(["month", "week", "day"] as CalendarMode[]).map((mode) => (
            <button
              key={mode}
              type="button"
              onClick={() => setViewMode(mode)}
              aria-current={viewMode === mode ? "page" : undefined}
              className={cn(
                "rounded-md px-2 transition-colors",
                viewMode === mode
                  ? "bg-card text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {mode === "month" ? "月视图" : mode === "week" ? "周视图" : "日视图"}
            </button>
          ))}
        </div>

        <span className="hidden text-[10px] text-muted-foreground/60 tabular-nums sm:inline">
          {semester} · {semesterWeekLabel}
        </span>
      </div>

      {viewMode === "day" && (
        <div className="shrink-0 overflow-x-auto pb-1">
          <div className="grid min-w-[348px] select-none grid-cols-7">
            {DAY_SHORT.map((day, i) => {
              const dateKey = weekData ? dayCellKey(weekData.week_start_date, i) : ""
              const dateNum = dateKey ? dateKey.slice(8) : ""
              const isToday = dateKey === today
              const isSelected = i === selectedDayIndex

              return (
                <button
                  key={day}
                  type="button"
                  onClick={() => handleDayClick(i)}
                  className={cn(
                    "flex h-12 min-w-11 flex-col items-center justify-center text-center transition-colors",
                    isToday
                      ? "bg-primary text-primary-foreground shadow-sm"
                      : isSelected
                        ? "bg-primary/10 text-primary"
                        : "text-foreground hover:bg-muted/40",
                  )}
                >
                  <span className="text-[10px] font-medium leading-tight">{day}</span>
                  <span className="mt-0.5 text-xs font-semibold leading-tight tabular-nums">
                    {dateNum || "--"}
                  </span>
                </button>
              )
            })}
          </div>
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-y-auto pb-4">
        {viewMode !== "month" && loading && (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        )}

        {viewMode !== "month" && !loading && error && !weekData && (
          <div className="p-8 text-center">
            <p className="text-sm text-muted-foreground">{error}</p>
            <p className="mt-1 text-xs text-muted-foreground/60">
              请先导入课表，或先创建带截止日期的待办。
            </p>
            <button
              type="button"
              onClick={() => setWeekRefresh((n) => n + 1)}
              className="mt-3 inline-flex h-9 items-center rounded-md px-3 text-xs font-medium text-primary transition-colors hover:bg-primary/10"
            >
              重试
            </button>
          </div>
        )}

        {viewMode !== "month" && !loading && !error && !weekData && (
          <p className="py-8 text-center text-sm text-muted-foreground">暂无日历数据</p>
        )}

        {viewMode !== "month" && !loading && weekData && allEvents.length === 0 && (
          <div className="mx-auto max-w-sm px-6 py-10 text-center">
            <CalendarDays className="mx-auto h-8 w-8 text-muted-foreground/60" />
            <p className="mt-3 text-sm font-medium text-foreground">本周暂无安排</p>
            <p className="mt-1 text-xs text-muted-foreground">
              有截止日期的待办、课程和考试会显示在这里。
            </p>
          </div>
        )}

        {/* 月视图 */}
        {viewMode === "month" && monthLoading && (
          <div className="flex items-center justify-center py-12">
            <div className="h-6 w-6 animate-spin rounded-full border-2 border-primary border-t-transparent" />
          </div>
        )}

        {viewMode === "month" && !monthLoading && monthError && (
          <div className="p-8 text-center">
            <p className="text-sm text-muted-foreground">{monthError}</p>
            <button
              type="button"
              onClick={() => setMonthRefresh((n) => n + 1)}
              className="mt-3 inline-flex h-9 items-center rounded-md px-3 text-xs font-medium text-primary transition-colors hover:bg-primary/10"
            >
              重试
            </button>
          </div>
        )}

        {viewMode === "month" && !monthLoading && !monthError && monthWeeksData.length > 0 && (
          <>
            {monthEventCount === 0 && (
              <div className="mx-auto max-w-sm px-6 pb-3 pt-6 text-center">
                <CalendarDays className="mx-auto h-8 w-8 text-muted-foreground/60" />
                <p className="mt-3 text-sm font-medium text-foreground">{monthTitle}暂无安排</p>
                <p className="mt-1 text-xs text-muted-foreground">
                  仍可点击日期查看当天详情。
                </p>
              </div>
            )}
            <CalendarMonthGrid
              gridDays={monthGridDays}
              eventsByDate={monthEventsByDate}
              today={today}
              selectedDateKey={selectedDateKey}
              onDateClick={handleMonthDateClick}
            />
          </>
        )}

        {viewMode === "month" && !monthLoading && !monthError && monthWeeksData.length === 0 && (
          <div className="mx-auto max-w-sm px-6 py-10 text-center">
            <CalendarDays className="mx-auto h-8 w-8 text-muted-foreground/60" />
            <p className="mt-3 text-sm font-medium text-foreground">暂无月视图数据</p>
            <p className="mt-1 text-xs text-muted-foreground">
              请重试加载日历数据。
            </p>
          </div>
        )}

        {!loading && weekData && allEvents.length > 0 && viewMode === "week" && (
          <>
            <div className="hidden md:block">
              <CalendarWeekTimetable
                weekData={weekData}
                allEvents={allEvents}
                today={today}
                selectedDayIndex={selectedDayIndex}
                onDayClick={handleDayClick}
                onEventClick={handleEventClick}
              />
            </div>
            <div className="md:hidden">
              <CalendarWeekTimetable
                weekData={weekData}
                allEvents={allEvents}
                today={today}
                selectedDayIndex={selectedDayIndex}
                compact
                onDayClick={handleDayClick}
                onEventClick={handleEventClick}
              />
            </div>
          </>
        )}

        {!loading && weekData && allEvents.length > 0 && viewMode === "day" && (
          <div className="px-1 pb-4">
            {selectedEvents.length === 0 ? (
              <div className="px-4 py-10 text-center">
                <p className="text-sm font-medium text-foreground">
                  {selectedDate || "这一天"}暂无安排
                </p>
                <p className="mt-1 text-xs text-muted-foreground">
                  切换上方日期查看本周其他日程。
                </p>
              </div>
            ) : (
              <div className="mx-auto max-w-2xl space-y-4">
                <div className="px-2 pb-1 pt-2 text-xs font-medium text-muted-foreground">
                  {selectedDate} 周{DAY_SHORT[selectedDayIndex]}
                </div>

                {dayDailyEvents.length > 0 && (
                  <div className="space-y-2">
                    <div className="flex items-center gap-1.5 px-1 text-[11px] font-medium text-emerald-600 dark:text-emerald-400">
                      <Repeat className="h-3.5 w-3.5" />
                      每日习惯
                      <span className="ml-0.5 text-[10px] text-muted-foreground">
                        {dayDailyEvents.filter((e) => e.tags.includes("完成")).length}/{dayDailyEvents.length} 今日完成
                      </span>
                    </div>
                    {dayDailyEvents.map((event) => (
                      <EventCard
                        key={`day-daily-${event.id}`}
                        event={event}
                        onClick={() => handleEventClick(event)}
                      />
                    ))}
                  </div>
                )}

                {dayTodoEvents.length > 0 && (
                  <div className="space-y-2">
                    <div className="flex items-center gap-1.5 px-1 text-[11px] font-medium text-amber-600 dark:text-amber-400">
                      <ListTodo className="h-3.5 w-3.5" />
                      待办
                    </div>
                    {dayTodoEvents.map((event) => (
                      <EventCard
                        key={`day-todo-${event.id}`}
                        event={event}
                        onClick={() => handleEventClick(event)}
                      />
                    ))}
                  </div>
                )}

                {dayTimedEvents.length > 0 && (
                  <div className="space-y-2">
                    {(dayDailyEvents.length > 0 || dayTodoEvents.length > 0) && (
                      <div className="flex items-center gap-1.5 px-1 text-[11px] font-medium text-muted-foreground">
                        <CalendarDays className="h-3.5 w-3.5" />
                        日程
                      </div>
                    )}
                    {dayTimedEvents.map((event) => (
                      <EventCard
                        key={`day-${event.kind}-${event.id}`}
                        event={event}
                        onClick={() => handleEventClick(event)}
                      />
                    ))}
                  </div>
                )}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
