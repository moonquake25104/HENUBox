import { useState } from "react"
import { CalendarDays, Check, ChevronLeft, ChevronRight } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import "./plan-date.css"

const pad = (n: number) => String(n).padStart(2, "0")
const key = (year: number, month: number, day: number) => `${year}-${pad(month)}-${pad(day)}`
const label = (date: string) => { const [y, m, d] = date.split("-").map(Number); return `${y}年${m}月${d}日` }
function shanghaiToday() {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" }).formatToParts(new Date())
  return ["year", "month", "day"].map(type => parts.find(p => p.type === type)!.value).join("-")
}
function offset(date: string, days: number) { const [y, m, d] = date.split("-").map(Number), value = new Date(Date.UTC(y, m - 1, d + days)); return key(value.getUTCFullYear(), value.getUTCMonth() + 1, value.getUTCDate()) }

export function PlanDatePicker({ value, today = shanghaiToday(), disabled, change }: { value: string; today?: string; disabled: boolean; change: (date: string) => void }) {
  const [open, setOpen] = useState(false)
  return <>
    <div className="xt-date-field"><button className="xt-date-trigger" type="button" aria-label="计划日期" aria-haspopup="dialog" disabled={disabled} onClick={() => setOpen(true)}><CalendarDays size={17} /><span className={!value ? "is-placeholder" : undefined}>{value ? label(value) : "选择日期"}</span><ChevronRight size={16} /></button>{value && <button type="button" onClick={() => change("")} disabled={disabled}>清除日期</button>}</div>
    {open && <DateCalendar value={value} today={today} close={() => setOpen(false)} choose={date => { change(date); setOpen(false) }} />}
  </>
}

function DateCalendar({ value, today, close, choose }: { value: string; today: string; close: () => void; choose: (date: string) => void }) {
  const initial = value || today, [selected, setSelected] = useState(initial), [cursor, setCursor] = useState(initial.slice(0, 7)), [view, setView] = useState<"days" | "months" | "years">("days"), [yearStart, setYearStart] = useState(Math.max(2000, Math.min(2089, Number(initial.slice(0, 4)) - 5)))
  const [year, month] = cursor.split("-").map(Number), first = (new Date(Date.UTC(year, month - 1, 1)).getUTCDay() + 6) % 7, count = new Date(Date.UTC(year, month, 0)).getUTCDate()
  const move = (n: number) => { const d = new Date(Date.UTC(year, month - 1 + n, 1)); setCursor(`${d.getUTCFullYear()}-${pad(d.getUTCMonth() + 1)}`) }
  const quick = (days: number) => { const date = offset(today, days); setSelected(date); setCursor(date.slice(0, 7)); setView("days") }
  return <Modal open title="计划日期" description="为这件事，安排一个日子。" className="xt-calendar" layer={70} onOpenChange={open => { if (!open) close() }}>
    <div className="xt-calendar-selection"><CalendarDays size={19} /><span>{label(selected)}<small>只用于整理事项，不自动提醒</small></span></div>
    <div className="xt-calendar-quick">{[[0, "今天"], [1, "明天"], [7, "一周后"]].map(([days, text]) => <button type="button" key={days} onClick={() => quick(Number(days))}>{text}</button>)}</div>
    <div className="xt-calendar-nav"><button type="button" aria-label={view === "years" ? "前十二年" : "上个月"} disabled={view === "years" ? yearStart <= 2000 : cursor <= "2000-01"} onClick={() => view === "years" ? setYearStart(n => Math.max(2000, n - 12)) : move(-1)}><ChevronLeft size={19} /></button><div><button type="button" aria-label="选择年份" onClick={() => setView(view === "years" ? "days" : "years")}>{year}年</button><button type="button" aria-label="选择月份" onClick={() => setView(view === "months" ? "days" : "months")}>{month}月</button></div><button type="button" aria-label={view === "years" ? "后十二年" : "下个月"} disabled={view === "years" ? yearStart >= 2089 : cursor >= "2100-12"} onClick={() => view === "years" ? setYearStart(n => Math.min(2089, n + 12)) : move(1)}><ChevronRight size={19} /></button></div>
    {view === "days" ? <><div className="xt-calendar-week" aria-hidden="true">{["一", "二", "三", "四", "五", "六", "日"].map(day => <span key={day}>{day}</span>)}</div><div className="xt-calendar-days" role="group" aria-label={`${year}年${month}月日期`}>{Array.from({ length: 42 }, (_, i) => { const day = i - first + 1, date = key(year, month, day); return day > 0 && day <= count ? <button type="button" key={i} aria-label={label(date)} aria-pressed={date === selected} aria-current={date === today ? "date" : undefined} onClick={() => setSelected(date)}>{day}</button> : <span key={i} /> })}</div></> : <div className="xt-calendar-options">{Array.from({ length: 12 }, (_, i) => view === "months" ? i + 1 : yearStart + i).map(n => <button type="button" key={n} aria-pressed={n === (view === "months" ? month : year)} onClick={() => { setCursor(view === "months" ? `${year}-${pad(n)}` : `${n}-${pad(month)}`); setView(view === "years" ? "months" : "days") }}>{n}{view === "months" ? "月" : "年"}</button>)}</div>}
    <div className="xt-calendar-actions"><button type="button" onClick={() => choose("")}>不设日期</button><button type="button" onClick={close}>取消</button><button type="button" className="xt-primary" onClick={() => choose(selected)}><Check size={16} />确定日期</button></div>
  </Modal>
}
