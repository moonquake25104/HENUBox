import { useRef, useState, type CSSProperties } from "react"
import { useContentTransition } from "@/hooks/use-content-transition"
import { ArrowLeft, BarChart3, Check, ChevronRight, Pause, Play, Plus, Settings2, Square, Timer, Trash2 } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import { useMobileBack } from "@/lib/mobile/back"
import { FOCUS_NAMES, focusDuration, focusTime, type FocusMode, type FocusSettings } from "@/lib/mobile/focus"
import type { FocusController } from "@/hooks/use-focus"
import "./focus.css"
import { FocusGoalProgress, FocusGoalSettings, FocusHeatmap } from "./FocusInsights"
import { FocusExtrasPanel } from "./FocusExtrasPanel"
import { ChoicePicker } from "./ChoicePicker"

export function FocusPage({ focus, courseNames, close }: { focus: FocusController; courseNames: string[]; close: () => void }) {
  const [view, setView] = useState<"timer" | "records" | "settings">("timer")
  const [choice, setChoice] = useState<FocusMode | null>(null)
  const [label, setLabel] = useState("")
  const [confirm, setConfirm] = useState(false)
  const [tag, setTag] = useState("")
  const panel = useRef<HTMLElement>(null)
  useContentTransition(panel, view, view === "timer" ? 0 : 1, !confirm, true)
  useMobileBack(true, () => view === "timer" ? close() : setView("timer"), 10)
  const { state: s, busy, error } = focus
  const t = s?.timer, mode = t?.mode ?? choice ?? s?.next_mode ?? "focus"
  const planned = mode === "countup" ? 0 : (s?.settings[mode === "focus" ? "focus_minutes" : mode === "short_break" ? "short_minutes" : "long_minutes"] ?? 25) * 60000
  const time = t ? mode === "countup" ? t.elapsed_ms : t.remaining_ms : planned
  const progress = t ? mode === "countup" ? (t.elapsed_ms % 3600000) / 3600000 : Math.min(1, t.elapsed_ms / t.planned_ms) : 0
  const candidates = [...new Set([...(s?.labels ?? []), ...courseNames.filter(n => [...n].length <= 40)])]
  const running = t?.status === "RUNNING"
  const doStart = async () => { if (await focus.command("start", mode, label)) setChoice(null) }
  return <section ref={panel} className="xm-focus-page" aria-label="专注时光">
    <header className="xm-focus-header">
      <button aria-label={view === "timer" ? "返回校园" : "返回计时"} onClick={() => view === "timer" ? close() : setView("timer")}><ArrowLeft size={21} /></button>
      <div><h1>{view === "records" ? "专注记录" : view === "settings" ? "专注设置" : "专注时光"}</h1><p>{view === "timer" ? "把这一刻，留给眼前的事。" : "保存在手机，离线也能查看。"}</p></div>
      {view === "timer" && <><button aria-label="查看专注记录" onClick={() => setView("records")}><BarChart3 size={21} /></button><button aria-label="打开专注设置" onClick={() => setView("settings")}><Settings2 size={21} /></button></>}
    </header>
    {error && <div className="xm-notice" role="alert">{error}<button onClick={() => void focus.refresh()}>刷新状态</button></div>}
    {!s ? <p className="xf-muted">正在读取本机专注数据…</p> : <>
      {view === "timer" && <>
        <div className="xf-modes" aria-label="计时模式">{(["focus", "countup", "short_break", "long_break"] as FocusMode[]).map(m => <button key={m} aria-pressed={mode === m} disabled={!!t || busy} onClick={() => setChoice(m)}>{FOCUS_NAMES[m]}</button>)}</div>
        <div className={`xf-dial ${mode.includes("break") ? "is-break" : ""}`} style={{ "--focus-progress": `${progress * 360}deg` } as CSSProperties}>
          <div className="xf-dial-content"><span>{t ? running ? FOCUS_NAMES[mode] : t.status === "INTERRUPTED" ? "计时已中断" : "已暂停" : "准备好了，就开始吧"}</span><strong role="timer" aria-label="计时时间" aria-live="off">{focusTime(time)}</strong><small>{t?.label || label || "自由安排这一段时间"}</small><em>{mode === "countup" ? "正向累计 · 最长 24 小时" : `已完成 ${s.round} / ${s.settings.long_every} 轮`}</em></div>
        </div>
        {!t && <div className="xf-label">这次专注什么<ChoicePicker title="专注标签" value={label} options={[{ value: "", label: "未分类" }, ...candidates.map(n => ({ value: n, label: n }))]} choose={setLabel} disabled={busy} /></div>}
        {s.interruption && <p className="xf-hint">手机重启导致计时中断，已保留最近保存的时长。你可以继续，或结束并保存；关机时长不会计入。</p>}
        <div className="xf-controls"><button className="xf-primary" disabled={busy} onClick={() => t ? void focus.command(running ? "pause" : "resume") : void doStart()}>{running ? <Pause size={19} /> : <Play size={19} />}{t ? running ? "暂停" : "继续计时" : mode.includes("break") ? "开始休息" : "开始专注"}</button>{t && <button className="xf-secondary" disabled={busy} onClick={() => setConfirm(true)}><Square size={16} />结束</button>}</div>
        {!t && s.last_finished && <p className="xf-completed"><Check size={15} />上一段已保存 · {focusDuration(s.last_finished.actual_ms / 1000)}{s.last_finished.completed ? " · 完成一个番茄" : ""}</p>}
        {!s.notification_permission && <p className="xf-hint">尚未允许通知，后台计时仍可使用，但通知栏和到点提醒可能不可见。可在系统设置中允许HENUBox通知。</p>}
        <button className="xf-today" onClick={() => setView("records")}><span><small>今日已记录</small><strong>{focusDuration(s.today.seconds)}</strong></span><span><small>完成番茄</small><strong>{s.today.pomodoros} 个</strong></span><ChevronRight size={19} /></button>
        <FocusGoalProgress seconds={s.today.seconds} />
        <button className="xf-secondary xf-environment-link" onClick={() => setView("settings")}><Settings2 size={16} />背景音 · 勿扰</button>
        <p className="xf-footnote">暂停和休息不计入专注。返回、锁屏可继续计时；系统强制停止可能影响到点提醒。</p>
      </>}
      {view === "settings" && <>
        <FocusSettingsForm key={s.revision + ":" + !!t} value={s.settings} active={!!t} busy={busy} save={focus.saveSettings} />
        <FocusGoalSettings />
        <FocusExtrasPanel />
        <section className="xf-panel"><h2>我的标签</h2><p className="xf-muted">课表中的课程也能直接选择。归档不会删除历史记录。</p><form className="xf-tag-form" onSubmit={e => { e.preventDefault(); void focus.command("tag", "", tag).then(ok => { if (ok) setTag("") }) }}><input aria-label="新标签名称" value={tag} maxLength={80} placeholder="例如：英语、阅读" onChange={e => setTag(e.target.value)} /><button disabled={busy || !tag.trim() || [...tag.trim()].length > 40} aria-label="添加专注标签"><Plus size={20} /></button></form><div className="xf-tags">{s.labels.map(n => <span key={n}>{n}<button aria-label={`归档标签 ${n}`} disabled={busy} onClick={() => void focus.command("archive", "", n)}><Trash2 size={14} /></button></span>)}</div></section>
      </>}
      {view === "records" && <>
        <div className="xf-stats-hero"><Timer size={25} /><small>今天的专注</small><strong>{focusDuration(s.today.seconds)}</strong><span>完成 {s.today.pomodoros} 个番茄</span></div>
        <FocusHeatmap heatmap={s.heatmap} />
        <section className="xf-panel"><h2>本周节奏</h2><div className="xf-week">{s.week.map((d, i) => <div key={d.date} aria-label={`${d.date} ${focusDuration(d.seconds)}`}><small>{Math.floor(d.seconds / 60)}分</small><span style={{ height: `${Math.max(3, d.seconds / Math.max(60, ...s.week.map(v => v.seconds)) * 76)}px` }} /><b>{["一", "二", "三", "四", "五", "六", "日"][i]}</b></div>)}</div><p className="xf-muted">本周共 {focusDuration(s.week.reduce((n, d) => n + d.seconds, 0))} · 按开始日期归档</p></section>
        <section className="xf-panel xf-history"><h2>最近记录</h2>{!s.records.length && <p className="xf-muted">还没有记录，先专注一小段时间吧。</p>}{s.records.map(r => <article key={r.id}><div><strong>{r.label || FOCUS_NAMES[r.mode]}</strong><small>{new Date(r.started_at).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai", month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" })} · {r.mode.includes("break") ? "休息，不计入统计" : r.completed ? "完整番茄" : r.mode === "countup" ? "自由专注" : "提前结束"}</small></div><b>{focusDuration(r.actual_ms / 1000)}</b></article>)}{s.records.length >= 100 && <p className="xf-muted">显示最近 100 条，统计包含全部记录。</p>}</section>
      </>}
    </>}
    {confirm && <Modal open title="结束这次计时？" onOpenChange={setConfirm} className="xm-feature-modal"><p className="xf-muted">已用时长会保存。提前结束不计作一个完整番茄；休息不会计入专注时长。</p><div className="xf-controls"><button className="xf-secondary" disabled={busy} onClick={() => setConfirm(false)}>继续这一段</button><button className="xf-primary" disabled={busy} onClick={() => void focus.command("finish").then(ok => { if (ok) setConfirm(false) })}>结束并保存</button></div></Modal>}
  </section>
}

function FocusSettingsForm({ value, active, busy, save }: { value: FocusSettings; active: boolean; busy: boolean; save: (v: FocusSettings) => Promise<boolean> }) {
  const [p, setP] = useState(value), [saved, setSaved] = useState(false)
  const numbers = [{ key: "focus_minutes", title: "专注", max: 180 }, { key: "short_minutes", title: "短休息", max: 60 }, { key: "long_minutes", title: "长休息", max: 60 }, { key: "long_every", title: "长休间隔（轮）", max: 12 }] as const
  const valid = numbers.every(n => Number.isInteger(p[n.key]) && p[n.key] >= 1 && p[n.key] <= n.max)
  return <form className="xf-panel" onSubmit={e => { e.preventDefault(); void save(p).then(ok => setSaved(ok)) }}>
    <h2>自己的节奏</h2><p className="xf-muted">时长单位为分钟，每完成指定轮数后进入长休息。</p>
    <div className="xf-presets"><button type="button" disabled={active || busy} onClick={() => setP({ ...p, focus_minutes: 25, short_minutes: 5, long_minutes: 15, long_every: 4 })}>经典 25 / 5</button><button type="button" disabled={active || busy} onClick={() => setP({ ...p, focus_minutes: 50, short_minutes: 10, long_minutes: 20, long_every: 3 })}>深度 50 / 10</button></div>
    <div className="xf-fields">{numbers.map(n => <label key={n.key}>{n.title}<input type="number" inputMode="numeric" aria-label={n.title} min={1} max={n.max} disabled={active || busy} value={p[n.key] || ""} onChange={e => { setSaved(false); setP({ ...p, [n.key]: Number(e.target.value) }) }} /></label>)}</div>
    {([{ key: "auto_break", title: "专注结束自动开始休息" }, { key: "auto_focus", title: "休息结束自动开始专注" }, { key: "sound", title: "到点声音提醒" }, { key: "vibrate", title: "到点振动提醒" }] as const).map(n => <label className="xf-switch" key={n.key}><span>{n.title}</span><input type="checkbox" role="switch" checked={p[n.key]} disabled={active || busy} onChange={e => { setSaved(false); setP({ ...p, [n.key]: e.target.checked }) }} /></label>)}
    {active && <p className="xf-hint">请结束当前计时后再修改设置。</p>}<button className="xf-primary" disabled={active || busy || !valid}>{saved ? "已保存" : "保存专注设置"}</button>
  </form>
}
