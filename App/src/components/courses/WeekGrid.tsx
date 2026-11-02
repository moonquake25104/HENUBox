import type { WeekScheduleItem, WeekScheduleResponse } from "@/types/schedule"
import { cn } from "@/lib/utils"
import { GraduationCap } from "lucide-react"
import {
  TOTAL_HOURS,
  HOUR_HEIGHT,
  START_HOUR,
  timeToMinutes,
  minutesToTop,
  isDarkColor,
  hexToRgba,
  dayCellKey,
} from "./utils"

interface WeekGridProps {
  weekData: WeekScheduleResponse
  today: string
  mobileDayIndex: number
  onBlockClick: (item: WeekScheduleItem) => void
}

export function WeekGrid({ weekData, today, mobileDayIndex, onBlockClick }: WeekGridProps) {
  return (
    <>
      {/* Desktop: 7-column grid */}
      <div className="hidden md:block overflow-x-auto">
        <div
          className="grid select-none"
          style={{
            gridTemplateColumns: "48px repeat(7, minmax(0, 1fr))",
            minWidth: "680px",
          }}
        >
          {/* Time axis column */}
          <div className="relative bg-card" style={{ height: `${TOTAL_HOURS * HOUR_HEIGHT}px` }}>
            {Array.from({ length: TOTAL_HOURS }, (_, i) => {
              if (i === 0) return <div key={i} style={{ height: `${HOUR_HEIGHT}px` }} />
              return (
                <div
                  key={i}
                  className="absolute right-1.5 text-[10px] text-muted-foreground tabular-nums"
                  style={{ top: `${i * HOUR_HEIGHT - 6}px` }}
                >
                  {`${String(START_HOUR + i).padStart(2, "0")}:00`}
                </div>
              )
            })}
          </div>

          {/* 7 day columns */}
          {Array.from({ length: 7 }, (_, dayIdx) => {
            const dateKey = dayCellKey(weekData.week_start_date, dayIdx)
            const isTodayCol = dateKey === today
            const isSelCol = dayIdx === mobileDayIndex
            const dayItems = (weekData.items ?? []).filter(
              (it) => it.day_of_week === dayIdx + 1,
            )
            return (
              <div
                key={dayIdx}
                className={cn(
                  "relative border-l border-border/25",
                  isTodayCol && "bg-primary/[0.04]",
                  isSelCol && !isTodayCol && "bg-primary/[0.02]",
                )}
                style={{ height: `${TOTAL_HOURS * HOUR_HEIGHT}px` }}
              >
                {/* Hour grid lines */}
                {Array.from({ length: TOTAL_HOURS }, (_, i) => (
                  <div
                    key={i}
                    className="absolute inset-x-0 border-t border-border/25"
                    style={{ top: `${i * HOUR_HEIGHT}px` }}
                  />
                ))}

                {/* Course / exam blocks */}
                {dayItems.map((item) => {
                  const startMin = timeToMinutes(item.start_time)
                  const endMin = timeToMinutes(item.end_time)
                  const top = minutesToTop(startMin)
                  const height = Math.max(
                    24,
                    ((endMin - startMin) / 60) * HOUR_HEIGHT - 2,
                  )
                  const isExam = item.kind === "exam"
                  const dark = isDarkColor(item.color)

                  return (
                    <button
                      key={`${item.kind}-${item.id}`}
                      type="button"
                      onClick={() => onBlockClick(item)}
                      title={`${item.title}${item.location ? ` — ${item.location}` : ""}`}
                      className={cn(
                        "absolute left-0.5 right-0.5 overflow-hidden rounded-lg px-1 py-px text-left transition-all flex flex-col justify-center",
                        "hover:z-20 hover:shadow-lg",
                        isExam
                          ? "border-2 border-dashed cursor-default"
                          : "cursor-pointer hover:brightness-105",
                        dark ? "text-white" : "text-foreground",
                      )}
                      style={{
                        top: `${top}px`,
                        height: `${height}px`,
                        backgroundColor: isExam
                          ? hexToRgba(item.color, 0.16)
                          : item.color,
                        borderColor: isExam ? item.color : undefined,
                      }}
                    >
                      <span className="line-clamp-2 text-[11px] font-semibold leading-tight">
                        {isExam && <GraduationCap className="inline h-3 w-3 shrink-0 mr-0.5 -mt-px" />}
                        {item.title}
                      </span>
                      {item.location && height >= 40 && (
                        <span className="truncate text-[9px] leading-tight opacity-65">
                          {item.location}
                        </span>
                      )}
                      {height < 40 && (
                        <span className="sr-only">{item.title} — {item.location}</span>
                      )}
                    </button>
                  )
                })}
              </div>
            )
          })}
        </div>
      </div>

      {/* Mobile: fit all seven days inside the viewport */}
      <div className="overflow-x-hidden md:hidden">
        <div
          className="grid w-full select-none"
          style={{
            gridTemplateColumns: "28px repeat(7, minmax(0, 1fr))",
          }}
        >
          {/* Time axis column (sticky left) */}
          <div className="relative bg-card" style={{ height: `${TOTAL_HOURS * HOUR_HEIGHT}px` }}>
            {Array.from({ length: TOTAL_HOURS }, (_, i) => {
              if (i === 0) return <div key={i} style={{ height: `${HOUR_HEIGHT}px` }} />
              return (
                <div
                  key={i}
                  className="absolute right-1 text-[9px] text-muted-foreground tabular-nums"
                  style={{ top: `${i * HOUR_HEIGHT - 5}px` }}
                >
                  {`${String(START_HOUR + i).padStart(2, "0")}`}
                </div>
              )
            })}
          </div>

          {/* 7 day columns */}
          {Array.from({ length: 7 }, (_, dayIdx) => {
            const dateKey = dayCellKey(weekData.week_start_date, dayIdx)
            const isTodayCol = dateKey === today
            const isSelCol = dayIdx === mobileDayIndex
            const dayItems = (weekData.items ?? []).filter((it) => it.day_of_week === dayIdx + 1)
            return (
              <div
                key={dayIdx}
                className={cn(
                  "relative border-l border-border/25",
                  isTodayCol && "bg-primary/[0.06]",
                  isSelCol && "bg-primary/[0.03]",
                )}
                style={{ height: `${TOTAL_HOURS * HOUR_HEIGHT}px` }}
              >
                {/* Hour grid lines */}
                {Array.from({ length: TOTAL_HOURS }, (_, i) => (
                  <div
                    key={i}
                    className="absolute inset-x-0 border-t border-border/25"
                    style={{ top: `${i * HOUR_HEIGHT}px` }}
                  />
                ))}

                {/* Course / exam blocks */}
                {dayItems.map((item) => {
                  const startMin = timeToMinutes(item.start_time)
                  const endMin = timeToMinutes(item.end_time)
                  const top = minutesToTop(startMin)
                  const height = Math.max(26, ((endMin - startMin) / 60) * HOUR_HEIGHT - 2)
                  const isExam = item.kind === "exam"
                  const dark = isDarkColor(item.color)

                  return (
                    <button
                      key={`m-${item.kind}-${item.id}`}
                      type="button"
                      onClick={() => onBlockClick(item)}
                      title={item.title}
                      className={cn(
                        "absolute left-px right-px overflow-hidden rounded-lg px-1 py-px text-left transition-all active:brightness-90 flex items-center",
                        isExam ? "border border-dashed" : "",
                        dark ? "text-white" : "text-foreground",
                      )}
                      style={{
                        top: `${top}px`,
                        height: `${height}px`,
                        backgroundColor: isExam ? hexToRgba(item.color, 0.16) : item.color,
                        borderColor: isExam ? item.color : undefined,
                      }}
                    >
                      <span className="line-clamp-2 text-[10px] font-semibold leading-tight w-full">
                        {item.title}
                      </span>
                    </button>
                  )
                })}
              </div>
            )
          })}
        </div>
      </div>
    </>
  )
}
