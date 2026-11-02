import { useEffect, useRef, useState } from "react"
import { BellRing, Clock3, Settings2 } from "lucide-react"
import { courseReminders, courseRemindersAvailable, type CourseReminderState } from "@/lib/mobile/course-reminders"

export function CourseReminderSettings() {
  const [state, setState] = useState<CourseReminderState | null>(null), [minutes, setMinutes] = useState("10"), [busy, setBusy] = useState(false), [error, setError] = useState("")
  const dirty = useRef(false)
  const supported = courseRemindersAvailable()
  const apply = (value: CourseReminderState) => { dirty.current = false; setState(value); setMinutes(String(value.minutes)) }
  useEffect(() => {
    if (!supported) return
    let live = true
    const load = () => { void courseReminders.getState().then(value => { if (live) { setState(value); if (!dirty.current) setMinutes(String(value.minutes)) } }).catch(() => { if (live) setError("提醒设置暂时无法读取，请重新打开。") }) }
    const visible = () => { if (!document.hidden) load() }; load(); document.addEventListener("visibilitychange", visible); window.addEventListener("focus", load)
    return () => { live = false; document.removeEventListener("visibilitychange", visible); window.removeEventListener("focus", load) }
  }, [supported])
  async function save(enabled: boolean, disabling = false) {
    if (!disabling && (!/^\d{1,3}$/.test(minutes) || Number(minutes) > 120)) { setError("请输入 0–120 分钟，0 表示上课时提醒。"); return }
    const lead = disabling ? state?.minutes ?? 10 : Number(minutes)
    setBusy(true); setError(""); try { apply(await courseReminders.configure({ enabled, minutes: lead })) } catch { setError("提醒设置未能保存，请稍后重试。") } finally { setBusy(false) }
  }
  const next = state?.next_at ? new Date(state.next_at).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai", month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" }) : "当前没有待提醒的课程"
  return <div className="xp-reminders">
    <div className="xp-option"><h3><BellRing size={18} />上课提醒</h3><p>按本机课表发送手机通知，断网也可提醒。更新、编辑课表后自动重新安排。</p><label className="xp-reminder-toggle"><span>开启上课提醒</span><input type="checkbox" role="switch" aria-label="开启上课提醒" checked={state?.enabled ?? false} disabled={busy || !supported || !state} onChange={e => void save(e.target.checked, !e.target.checked)} /></label></div>
    <div className="xp-option"><h3><Clock3 size={18} />提前多久</h3><div className="xp-reminder-minutes"><input aria-label="提前分钟数" type="number" inputMode="numeric" min="0" max="120" value={minutes} disabled={busy || !supported} onChange={e => { dirty.current = true; setMinutes(e.target.value) }} /><span>分钟</span><button className="xp-reminder-save" disabled={busy || !supported || !state} onClick={() => void save(state?.enabled ?? false)}>保存</button></div><div className="xp-reminder-presets">{[5,10,15,30].map(n => <button key={n} aria-pressed={minutes === String(n)} onClick={() => { dirty.current = true; setMinutes(String(n)) }}>{n} 分钟</button>)}</div></div>
    {state && <div className="xp-option"><h3><Settings2 size={18} />提醒状态</h3><p role="status">{!state.enabled ? "尚未开启" : !state.notification_permission ? "通知权限未开启，当前不会发送提醒。" : state.schedule_error ? "提醒未能安排，请重新保存设置。" : `已开启 · ${next}`}</p>{!state.notification_permission && <button className="xp-reminder-link" onClick={() => void courseReminders.openNotificationSettings().catch(() => setError("暂时无法打开系统设置。"))}>去系统开启通知</button>}{state.enabled && state.notification_permission && !state.exact_alarm && <><p>系统尚未允许准时提醒，通知可能延迟。你可以选择授权“闹钟和提醒”。</p><button className="xp-reminder-link" onClick={() => void courseReminders.openExactSettings().catch(() => setError("暂时无法打开系统设置。"))}>设置准时提醒</button></>}</div>}
    {!supported && <p className="xp-caption">手机 App 更新后可使用上课提醒。</p>}{error && <p className="xp-error" role="alert">{error}</p>}
  </div>
}
