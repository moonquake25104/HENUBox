import type { WeekScheduleItem } from "@/types/schedule"
import { useEffect, useState, type CSSProperties } from "react"
import { Plus } from "lucide-react"
import { useMobileBack } from "@/lib/mobile/back"
import { chinaDate, datePlus, weekdays } from "@/components/xique/dates"
import { localWorkspaceBridge } from "@/lib/mobile/local-school"
import {
  addBreaks,
  clockLabel,
  clockMinutes,
  configuredSlots,
  slotPosition,
  slotTotal,
  weekSlots,
  type TimeSlot,
} from "@/lib/mobile/week-timeline"

interface Lane {
  item: WeekScheduleItem
  lane: number
  lanes: number
}
interface Props {
  days: Lane[][]
  weekStart: string
  open: (item: WeekScheduleItem) => void
  workspaceId?: string
  semester: string
  revision?: number
  add?: (slot: { day_of_week: number; start_time: string; end_time: string }) => void
}

// Fit the actual time range, including late and overlapping courses, to the
// available screen height. Only the presentation is compressed; times stay exact.
function ink(color: string) {
  if (!/^#[0-9a-f]{6}$/i.test(color)) return "#17212d"
  const rgb = [1, 3, 5]
    .map((index) => parseInt(color.slice(index, index + 2), 16) / 255)
    .map((value) => (value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4))
  return rgb[0] * 0.2126 + rgb[1] * 0.7152 + rgb[2] * 0.0722 > 0.179 ? "#101820" : "#ffffff"
}

export function CompactWeek({ days, weekStart, open, workspaceId, semester, revision, add }: Props) {
  const key = `${workspaceId}:${semester}`
  const selectionScope = `${key}:${weekStart}:${revision}:${days.length}`
  const [selected, setSelected] = useState<string | null>(null)
  const hasSelection = selected?.startsWith(selectionScope + "|") ?? false
  useMobileBack(hasSelection, () => setSelected(null))
  useEffect(() => {
    const clear = (event: PointerEvent) => { if (!(event.target instanceof Element) || !event.target.closest(".xm-week-add")) setSelected(null) }
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") setSelected(null) }
    document.addEventListener("pointerdown", clear)
    document.addEventListener("keydown", escape)
    return () => { document.removeEventListener("pointerdown", clear); document.removeEventListener("keydown", escape) }
  }, [])
  const [settings, setSettings] = useState<{ key: string; slots: TimeSlot[] }>({
    key: "",
    slots: [],
  })
  useEffect(() => {
    let active = true
    if (workspaceId)
      void localWorkspaceBridge
        .getSettings({ workspace_id: workspaceId, semester })
        .then((value) => {
          if (active)
            setSettings({
              key,
              slots: configuredSlots(value.settings.periods, value.settings.period_count),
            })
        })
        .catch(() => {
          /* Keep last known settings and all saved courses readable. */
        })
    return () => {
      active = false
    }
  }, [workspaceId, semester, revision, key])
  const items = days.flat().map((entry) => entry.item)
  const slots = addBreaks(weekSlots(settings.key === key ? settings.slots : [], items))
  const count = Math.max(1, slotTotal(slots))
  const position = (time: number) => (100 * slotPosition(slots, time)) / count
  return (
    <div className="xm-compact-week" aria-label="整周课表" data-week-days={days.length} style={{ "--xm-week-days": days.length } as CSSProperties}>
      <div className="xm-week-head">
        <span className="xm-axis-heading">时间</span>
        {weekdays.slice(0, days.length).map((label, index) => (
          <div key={label} className={datePlus(weekStart, index) === chinaDate() ? "is-today" : ""}>
            <span>{label}</span>
            <strong>{datePlus(weekStart, index).slice(5).replace("-", "/")}</strong>
          </div>
        ))}
      </div>
      <div className="xm-week-body">
        {slots.map((slot) =>
          slot.break ? (
            <div
              className={`xm-break-band xm-break-${slot.break}`}
              key={`break-${slot.break}`}
              style={{
                top: `${position(slot.start)}%`,
                height: `${position(slot.end) - position(slot.start)}%`,
              }}
              aria-label={slot.break === "lunch" ? "午休" : "晚休"}
            >
              {slot.break === "lunch" ? "午休" : "晚休"}
            </div>
          ) : null,
        )}
        {slots.map((slot) => (
          <span
            className="xm-grid-line"
            key={`line-${slot.start}-${slot.end}`}
            style={{ top: `${position(slot.end)}%` }}
            aria-hidden="true"
          />
        ))}
        <div className="xm-week-axis xm-period-axis">
          {slots.map((slot) =>
            slot.break ? (
              <div
                className="xm-break-time"
                key={`break-axis-${slot.break}`}
                aria-hidden="true"
                style={{
                  top: `${position(slot.start)}%`,
                  height: `${position(slot.end) - position(slot.start)}%`,
                }}
              />
            ) : (
              <div
                className="xm-period-time"
                key={`${slot.start}-${slot.end}`}
                data-start={clockLabel(slot.start)}
                data-end={clockLabel(slot.end)}
                aria-label={`${slot.index ? `第${slot.index}节` : "课外时段"} ${clockLabel(slot.start)}–${clockLabel(slot.end)}`}
                style={{
                  top: `${position(slot.start)}%`,
                  height: `${position(slot.end) - position(slot.start)}%`,
                }}
              >
                {slot.index && <b>{slot.index}</b>}
                <time>{clockLabel(slot.start)}</time>
                <time>{clockLabel(slot.end)}</time>
              </div>
            ),
          )}
        </div>
        {days.map((entries, index) => (
          <div
            key={index}
            className={`xm-week-day ${datePlus(weekStart, index) === chinaDate() ? "is-today" : ""}`}
          >
            {add && slots.filter(slot => !slot.break && !entries.some(({ item }) => clockMinutes(item.start_time) < slot.end && clockMinutes(item.end_time) > slot.start)).map(slot => {
              const id = `${selectionScope}|${index}:${slot.start}:${slot.end}`
              const active = selected === id
              return <button type="button" className={`xm-week-add${active ? " is-selected" : ""}`} key={`add-${slot.start}`}
                aria-label={`${weekdays[index]} ${clockLabel(slot.start)} ${active ? "确认添加课程" : "添加课程"}`}
                aria-pressed={active}
                title={active ? "再次点击添加课程" : "选择这个空白时段"}
                style={{ top: `${position(slot.start)}%`, height: `${position(slot.end) - position(slot.start)}%` }}
                onClick={() => {
                  if (!active) { setSelected(id); return }
                  setSelected(null)
                  add({ day_of_week: index + 1, start_time: clockLabel(slot.start), end_time: clockLabel(slot.end) })
                }}>{active && <Plus size={23} aria-hidden="true" />}</button>
            })}
            {entries.map(({ item, lane, lanes }) => (
              <button
                key={item.id}
                className="xm-week-course"
                style={{
                  top: `${position(clockMinutes(item.start_time))}%`,
                  height: `calc(${position(clockMinutes(item.end_time)) - position(clockMinutes(item.start_time))}% - 2px)`,
                  left: `calc(${(100 * lane) / lanes}% + 1px)`,
                  width: `calc(${100 / lanes}% - 2px)`,
                  backgroundColor: item.color,
                  color: ink(item.color),
                }}
                aria-label={`${weekdays[index]} ${item.title} ${item.start_time}–${item.end_time} ${item.location || "教室未提供"}`}
                title={`${item.title} ${item.start_time}–${item.end_time} ${item.location}`}
                onClick={() => { setSelected(null); open(item) }}
              >
                <strong>{item.title}</strong>
                <span>{item.location || "未标教室"}</span>
              </button>
            ))}
          </div>
        ))}
      </div>
    </div>
  )
}
