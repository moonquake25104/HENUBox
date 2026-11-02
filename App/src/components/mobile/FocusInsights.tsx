import { useState } from "react"
import { focusDuration, type FocusState } from "@/lib/mobile/focus"
import { useFocusGoal } from "@/lib/mobile/focus-goal"

export function FocusGoalProgress({ seconds }: { seconds: number }) {
  const { minutes } = useFocusGoal()
  if (!minutes) return null
  return <section className="xf-panel xf-goal-progress"><div><h2>今日目标</h2><strong>{Math.floor(seconds / 60)} / {minutes} 分钟</strong></div><progress aria-label="今日专注目标进度" max={minutes * 60} value={Math.min(seconds, minutes * 60)} /><p className="xf-muted">{seconds >= minutes * 60 ? "今天的目标完成了，按自己的节奏来。" : `还差 ${focusDuration(minutes * 60 - seconds)}，慢慢积累。`}仅统计已保存的专注时长。</p></section>
}
export function FocusGoalSettings() {
  const { minutes, setGoal } = useFocusGoal()
  return <GoalForm value={minutes} save={setGoal} />
}
function GoalForm({ value, save }: { value: number; save: (n: number) => void }) {
  const [enabled, setEnabled] = useState(value > 0), [draft, setDraft] = useState(value || 60), [saved, setSaved] = useState(false)
  return <form className="xf-panel" onSubmit={e => { e.preventDefault(); save(enabled ? draft : 0); setSaved(true) }}><h2>每日目标</h2><p className="xf-muted">只用于本机进度展示，不发送催促提醒，也不会自动开始计时。</p><label className="xf-switch"><span>启用每日目标</span><input type="checkbox" role="switch" checked={enabled} onChange={e => { setEnabled(e.target.checked); setSaved(false) }} /></label>{enabled && <label className="xf-goal-input">每天专注（分钟）<input type="number" inputMode="numeric" min={1} max={720} aria-label="每日目标分钟" value={draft || ""} onChange={e => { setDraft(Number(e.target.value)); setSaved(false) }} /></label>}<button className="xf-primary" disabled={enabled && (!Number.isInteger(draft) || draft < 1 || draft > 720)}>{saved ? "目标已保存" : "保存每日目标"}</button></form>
}
export function FocusHeatmap({ heatmap }: { heatmap: FocusState["heatmap"] }) {
  const [selected, setSelected] = useState("")
  if (!heatmap) return <section className="xf-panel"><h2>学习热力图</h2><p className="xf-muted">更新原生应用后即可读取全部本机记录。不会用最近 100 条记录推测完整历史。</p></section>
  const day = heatmap.days.find(d => d.date === selected) ?? heatmap.days.find(d => d.date === heatmap.today) ?? heatmap.days.at(-1)!
  const tier = (seconds: number) => seconds <= 0 ? 0 : seconds < 30 * 60 ? 1 : seconds < 60 * 60 ? 2 : seconds < 120 * 60 ? 3 : 4
  return <section className="xf-panel xf-heatmap-panel"><h2>学习热力图 <small>近 12 周</small></h2><div className="xf-heatmap" role="img" aria-label={`近12周已记录专注 ${focusDuration(heatmap.total_seconds)}，可在下方选择日期查看详细时长。`}>{heatmap.days.map(d => <span key={d.date} data-tier={tier(d.seconds)} data-future={d.date > heatmap.today || undefined} title={`${d.date} · ${focusDuration(d.seconds)}`} aria-hidden="true" />)}</div><div className="xf-heatmap-legend" aria-hidden="true"><span>少</span>{[0, 1, 2, 3, 4].map(i => <i key={i} data-tier={i} />)}<span>多</span></div><label className="xf-heatmap-date">查看每日专注<select aria-label="查看每日专注" value={day.date} onChange={e => setSelected(e.target.value)}>{heatmap.days.filter(d => d.date <= heatmap.today).toReversed().map(d => <option key={d.date} value={d.date}>{d.date} · {focusDuration(d.seconds)}</option>)}</select></label><p className="xf-muted">{day.date}：{focusDuration(day.seconds)} · {day.pomodoros} 个完整番茄。暂停和休息不计入。</p></section>
}
