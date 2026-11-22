import { useEffect, useState, type FormEvent, type ReactNode } from "react"
import { ArrowLeft, CalendarDays, ChevronLeft, ChevronRight, ExternalLink, ListChecks, Plus, Share2, Timer } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import { datePlus, monday, weekdays } from "@/components/xique/dates"
import { useLocalList } from "@/hooks/use-local-list"
import type { TodoController } from "@/hooks/use-todos"
import { useMobileBack } from "@/lib/mobile/back"
import { campusLink, countdownRemaining, dateDistance, openCampusLink, shareCampusText, validPlanDate, type Countdown } from "@/lib/mobile/campus-tools"
import type { TodoItem } from "@/lib/mobile/todos"
import { PlanDatePicker } from "./PlanDatePicker"
import { TodoEditor, TodoRow } from "./TodoPage"
import "./campus-tools.css"

function ToolsPage({ title, close, add, children, backLabel = "返回校园" }: {
  title: string; close: () => void; add?: () => void; children: ReactNode; backLabel?: string
}) {
  useMobileBack(true, close, 10)
  return <section className="xt-page xk-page" aria-label={title}>
    <header className="xt-header">
      <button aria-label={backLabel} onClick={close}><ArrowLeft size={21} /></button>
      <div><h1>{title}</h1></div>
      {add && <button aria-label="新建" onClick={add}><Plus size={22} /></button>}
    </header>
    {children}
  </section>
}

function Tabs<T extends string>({ label, options, selected, choose }: { label: string; options: readonly T[]; selected: T; choose: (value: T) => void }) {
  return <div className="xt-tabs" role="group" aria-label={label}>
    {options.map(value => <button type="button" key={value} aria-pressed={selected === value} onClick={() => choose(value)}>{value}</button>)}
  </div>
}

function RemoveButton({ remove }: { remove: () => boolean }) {
  const [confirm, setConfirm] = useState(false)
  return confirm ? <div className="xt-delete-confirm">
    <p>确定删除这条记录？</p>
    <button type="button" onClick={() => setConfirm(false)}>取消</button>
    <button type="button" onClick={() => { if (remove()) setConfirm(false) }}>确认删除</button>
  </div> : <button type="button" className="xt-delete" onClick={() => setConfirm(true)}>删除记录</button>
}

const categories = ["自习地点", "美食与视频", "校园消息"] as const
type Category = (typeof categories)[number]
type CampusNote = { id: string; category: Category; title: string; body: string; url: string }

function isCampusNote(value: unknown): value is CampusNote {
  const note = value as CampusNote | null
  return !!note && typeof note.id === "string" && categories.includes(note.category) &&
    typeof note.title === "string" && !!note.title && typeof note.body === "string" &&
    typeof note.url === "string" && (!note.url || campusLink(note.url) === note.url)
}

export function CampusSharePage({ close }: { close: () => void }) {
  const notes = useLocalList("henubox-campus-notes-v1", isCampusNote, 100)
  const [category, setCategory] = useState<"全部" | Category>("全部")
  const [edit, setEdit] = useState<CampusNote | "new" | null>(null)
  const [message, setMessage] = useState("")
  const [sharing, setSharing] = useState(false)
  const visible = notes.items.filter(note => category === "全部" || category === note.category)
  async function share(note: CampusNote) {
    setSharing(true)
    setMessage("")
    try {
      const copied = await shareCampusText([`【${note.category}】${note.title}`, note.body, note.url].filter(Boolean).join("\n"))
      if (copied) setMessage("已复制分享内容。")
    } catch (error) {
      if (!(error instanceof Error && error.name === "AbortError")) setMessage("暂时无法分享，请重试。")
    } finally {
      setSharing(false)
    }
  }
  return <ToolsPage title="校园信息分享" close={close} add={() => setEdit("new")}>
    <Tabs label="信息分类" options={["全部", ...categories]} selected={category} choose={setCategory} />
    <p className="xt-hint">收集长明教室、校园消息和抖音等美食视频链接，保存后可分享给同学。</p>
    {notes.error && <p className="xt-error" role="alert">{notes.error}</p>}
    {message && <p className="xt-hint" role="status">{message}</p>}
    {visible.length === 0 && <p className="xk-empty">还没有收藏的信息。</p>}
    {visible.map(note => <article key={note.id} className="xt-panel xk-card">
      <small>{note.category}</small>
      <h2>{note.title}</h2>
      {note.body && <p>{note.body}</p>}
      {note.url && <small className="xk-link">{new URL(note.url).hostname}</small>}
      <div className="xk-actions">
        <button onClick={() => void share(note)} disabled={sharing}><Share2 size={15} />分享</button>
        {note.url && <button onClick={() => { setMessage(""); void openCampusLink(note.url).catch(() => setMessage("暂时无法打开链接，请重试。")) }}><ExternalLink size={15} />打开链接</button>}
        <button onClick={() => setEdit(note)}>编辑</button>
      </div>
    </article>)}
    {edit && <CampusNoteEditor value={edit === "new" ? null : edit} category={category === "全部" ? categories[0] : category}
      error={notes.error} close={() => setEdit(null)} save={note => {
        if (!notes.save(note)) return
        if (category !== "全部") setCategory(note.category)
        setEdit(null)
      }} remove={id => { if (!notes.remove(id)) return false; setEdit(null); return true }} />}
  </ToolsPage>
}

function CampusNoteEditor({ value, category, error, close, save, remove }: {
  value: CampusNote | null; category: Category; error: string; close: () => void
  save: (note: CampusNote) => void; remove: (id: string) => boolean
}) {
  const [issue, setIssue] = useState("")
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const form = new FormData(event.currentTarget)
    const title = String(form.get("title")).trim()
    const body = String(form.get("body")).trim()
    const raw = String(form.get("url")).trim()
    const group = String(form.get("category")) as Category
    const url = campusLink(raw)
    if (!title) { setIssue("请填写标题。"); return }
    if (raw && !url) { setIssue("请填写有效的 HTTPS 链接，也可以粘贴含链接的抖音分享文字。"); return }
    setIssue("")
    save({ id: value?.id ?? crypto.randomUUID(), category: group, title, body, url })
  }
  return <Modal open title={value ? "编辑校园信息" : "记录校园信息"} className="xt-editor" onOpenChange={open => { if (!open) close() }}>
    <form onSubmit={submit}>
      <label>分类<select name="category" defaultValue={value?.category ?? category}>{categories.map(name => <option key={name}>{name}</option>)}</select></label>
      <label>标题<input name="title" defaultValue={value?.title} placeholder="例如：常去的自习室" maxLength={80} required /></label>
      <label>说明<textarea name="body" defaultValue={value?.body} placeholder="位置、开放时间或你想分享的内容" maxLength={1200} rows={3} /></label>
      <label>链接（可选）<textarea name="url" defaultValue={value?.url} placeholder="例如：粘贴抖音分享文字" maxLength={3000} rows={2} /></label>
      {(issue || error) && <p className="xt-error" role="alert">{issue || error}</p>}
      <button className="xt-primary" type="submit">保存校园信息</button>
      {value && <RemoveButton remove={() => remove(value.id)} />}
    </form>
  </Modal>
}

const plannerTabs = ["倒计时", "倒数日", "计划表"] as const
export type PlannerTab = (typeof plannerTabs)[number]
type DayEvent = { id: string; title: string; date: string }

function isCountdown(value: unknown): value is Countdown {
  const timer = value as Countdown | null
  return !!timer && typeof timer.id === "string" && typeof timer.title === "string" &&
    Number.isSafeInteger(timer.duration) && timer.duration > 0 && timer.duration < 24 * 3600000 &&
    Number.isSafeInteger(timer.remaining) && timer.remaining >= 0 && timer.remaining <= timer.duration &&
    (timer.endsAt === null || Number.isSafeInteger(timer.endsAt) && timer.endsAt > 0)
}

function isDayEvent(value: unknown): value is DayEvent {
  const item = value as DayEvent | null
  return !!item && typeof item.id === "string" && typeof item.title === "string" && validPlanDate(item.date)
}

function clockTime(ms: number) {
  const seconds = Math.ceil(ms / 1000)
  const hours = Math.floor(seconds / 3600)
  return [hours || null, Math.floor(seconds / 60) % 60, seconds % 60]
    .filter(value => value !== null).map(value => String(value).padStart(2, "0")).join(":")
}

export function CampusPlannerPage({ today, todos, close, initialTab, initialPlanDate, fromHome }: {
  today: string; todos: TodoController; close: () => void; initialTab: PlannerTab; initialPlanDate?: string; fromHome?: boolean
}) {
  const [tab, setTab] = useState(initialTab)
  return <ToolsPage title="时间与计划" close={close} backLabel={fromHome ? "返回首页" : "返回校园"}>
    <Tabs label="时间工具" options={plannerTabs} selected={tab} choose={setTab} />
    {tab === "倒计时" && <Countdowns />}
    {tab === "倒数日" && <DayEvents today={today} />}
    {tab === "计划表" && <WeeklyPlan todos={todos} today={today} initialDate={initialPlanDate} />}
  </ToolsPage>
}

function useCountdownClock(items: Countdown[]) {
  const [now, setNow] = useState(Date.now)
  const running = items.some(item => item.endsAt !== null && item.endsAt > now)
  useEffect(() => {
    if (!running) return
    const update = () => { if (!document.hidden) setNow(Date.now()) }
    const timer = window.setInterval(update, 250)
    document.addEventListener("visibilitychange", update)
    return () => { window.clearInterval(timer); document.removeEventListener("visibilitychange", update) }
  }, [running])
  return [now, setNow] as const
}

export function HomePlanner({ today, todos, open }: {
  today: string; todos: TodoController; open: (tab: PlannerTab, date?: string) => void
}) {
  const { items: countdowns } = useLocalList("henubox-countdowns-v1", isCountdown, 50)
  const { items: events } = useLocalList("henubox-day-events-v1", isDayEvent, 100)
  const [now] = useCountdownClock(countdowns)
  const timers = countdowns.filter(item => countdownRemaining(item, now) > 0 &&
    (item.endsAt !== null || item.remaining < item.duration)).slice(0, 3)
  const days = orderedDays(events, today).slice(0, 3)
  const plans = (todos.state?.items ?? []).filter(item => item.due_date && !item.completed)
    .sort((a, b) => a.due_date.localeCompare(b.due_date) || Number(b.important) - Number(a.important)).slice(0, 3)
  if (!timers.length && !days.length && !plans.length) return null
  return <section className="xk-home-planner" aria-label="时间与计划">
    {timers.length > 0 && <div className="xt-panel" aria-label="首页倒计时">
      <h2><Timer size={16} />倒计时</h2>
      {timers.map(item => <button key={item.id} onClick={() => open("倒计时")}>
        <span>{item.title}</span><time>{clockTime(countdownRemaining(item, now))}{item.endsAt === null ? " · 已暂停" : ""}</time>
        <ChevronRight size={15} />
      </button>)}
    </div>}
    {days.length > 0 && <div className="xt-panel" aria-label="首页倒数日">
      <h2><CalendarDays size={16} />倒数日</h2>
      {days.map(item => <button key={item.id} onClick={() => open("倒数日")}>
        <span>{item.title}</span><time dateTime={item.date}>{dayLabel(item.date, today)}</time><ChevronRight size={15} />
      </button>)}
    </div>}
    {plans.length > 0 && <div className="xt-panel" aria-label="首页计划表">
      <h2><ListChecks size={16} />计划表</h2>
      {plans.map(item => <button key={item.id} onClick={() => open("计划表", item.due_date)}>
        <span>{item.title}</span><time dateTime={item.due_date}>{item.due_date === today ? "今天" : item.due_date.slice(5)}</time>
        <ChevronRight size={15} />
      </button>)}
    </div>}
  </section>
}

function Countdowns() {
  const timers = useLocalList("henubox-countdowns-v1", isCountdown, 50)
  const [edit, setEdit] = useState<Countdown | "new" | null>(null)
  const [now, setNow] = useCountdownClock(timers.items)
  function toggle(item: Countdown, timestamp: number) {
    const remaining = countdownRemaining(item, timestamp)
    setNow(timestamp)
    if (item.endsAt !== null && remaining > 0) timers.save({ ...item, remaining, endsAt: null })
    else timers.save({ ...item, endsAt: timestamp + (remaining || item.duration) })
  }
  return <>
    <div className="xk-list-head"><h2>倒计时</h2><button onClick={() => setEdit("new")}><Plus size={16} />新建倒计时</button></div>
    {timers.error && <p className="xt-error" role="alert">{timers.error}</p>}
    {!timers.items.length && <p className="xk-empty">添加一段要计时的任务。</p>}
    {timers.items.map(item => {
      const left = countdownRemaining(item, now)
      const active = item.endsAt !== null && left > 0
      return <article key={item.id} className="xt-panel xk-card">
        <h2>{item.title}</h2>
        <strong className="xk-digits" role="timer" aria-label={`${item.title}剩余时间`}>{clockTime(left)}</strong>
        <small>{left === 0 ? "已到时" : active ? "计时中" : left === item.duration ? "待开始" : "已暂停"}</small>
        <div className="xk-actions">
          <button onClick={() => toggle(item, Date.now())}>{active ? "暂停" : left === 0 ? "重新开始" : left !== item.duration ? "继续" : "开始"}</button>
          <button onClick={() => timers.save({ ...item, endsAt: null, remaining: item.duration })}>重置</button>
          <button onClick={() => setEdit(item)}>编辑</button>
        </div>
      </article>
    })}
    <p className="xt-footnote">离开页面后按结束时间继续计算，到时会在这里显示。</p>
    {edit && <CountdownEditor value={edit === "new" ? null : edit} error={timers.error} close={() => setEdit(null)}
      save={item => { setNow(Date.now()); if (timers.save(item)) setEdit(null) }}
      remove={id => { if (!timers.remove(id)) return false; setEdit(null); return true }} />}
  </>
}

function CountdownEditor({ value, error, close, save, remove }: {
  value: Countdown | null; error: string; close: () => void
  save: (item: Countdown) => void; remove: (id: string) => boolean
}) {
  const [issue, setIssue] = useState("")
  return <Modal open title={value ? "编辑倒计时" : "新建倒计时"} className="xt-editor" onOpenChange={open => { if (!open) close() }}>
    <form onSubmit={event => {
      event.preventDefault()
      const form = new FormData(event.currentTarget)
      const title = String(form.get("title")).trim()
      const minutes = Number(form.get("minutes")), seconds = Number(form.get("seconds"))
      const duration = (minutes * 60 + seconds) * 1000
      if (!title || !Number.isInteger(minutes) || !Number.isInteger(seconds) ||
          minutes < 0 || minutes > 1439 || seconds < 0 || seconds > 59 || duration === 0) {
        setIssue("请填写标题和有效时长，上限为 23 小时 59 分 59 秒。")
        return
      }
      setIssue("")
      save(value && duration === value.duration ? { ...value, title } :
        { id: value?.id ?? crypto.randomUUID(), title, duration, remaining: duration, endsAt: null })
    }}>
      <label>标题<input name="title" defaultValue={value?.title} maxLength={80} required /></label>
      <label>分钟<input name="minutes" type="number" min={0} max={1439} required defaultValue={value ? Math.floor(value.duration / 60000) : 25} /></label>
      <label>秒<input name="seconds" type="number" min={0} max={59} required defaultValue={value ? value.duration / 1000 % 60 : 0} /></label>
      {value && <p className="xt-hint">修改时长会重置这一条计时。</p>}
      {(issue || error) && <p className="xt-error" role="alert">{issue || error}</p>}
      <button type="submit" className="xt-primary">保存倒计时</button>
      {value && <RemoveButton remove={() => remove(value.id)} />}
    </form>
  </Modal>
}

function orderedDays(events: DayEvent[], today: string) {
  const sorted = [...events].sort((a, b) => a.date.localeCompare(b.date))
  return [...sorted.filter(item => item.date >= today), ...sorted.filter(item => item.date < today).reverse()]
}

function dayLabel(date: string, today: string) {
  const left = dateDistance(date, today)
  return left === 0 ? "就是今天" : left > 0 ? `还有 ${left} 天` : `已过 ${-left} 天`
}

function DayEvents({ today }: { today: string }) {
  const events = useLocalList("henubox-day-events-v1", isDayEvent, 100)
  const [edit, setEdit] = useState<DayEvent | "new" | null>(null)
  const sorted = orderedDays(events.items, today)
  return <>
    <div className="xk-list-head"><h2>倒数日</h2><button onClick={() => setEdit("new")}><Plus size={16} />新建倒数日</button></div>
    {events.error && <p className="xt-error" role="alert">{events.error}</p>}
    {!sorted.length && <p className="xk-empty">记下考试、放假或其他重要日期。</p>}
    {sorted.map(item => {
      return <article key={item.id} className="xt-panel xk-card">
        <h2>{item.title}</h2>
        <strong className="xk-digits">{dayLabel(item.date, today)}</strong>
        <time dateTime={item.date}>{item.date}</time>
        <div className="xk-actions"><button onClick={() => setEdit(item)}>编辑</button></div>
      </article>
    })}
    {edit && <DayEventEditor value={edit === "new" ? null : edit} today={today} error={events.error} close={() => setEdit(null)}
      save={item => { if (events.save(item)) setEdit(null) }}
      remove={id => { if (!events.remove(id)) return false; setEdit(null); return true }} />}
  </>
}

function DayEventEditor({ value, today, error, close, save, remove }: {
  value: DayEvent | null; today: string; error: string; close: () => void
  save: (item: DayEvent) => void; remove: (id: string) => boolean
}) {
  const [date, setDate] = useState(value?.date ?? today)
  const [issue, setIssue] = useState("")
  return <Modal open title={value ? "编辑倒数日" : "新建倒数日"} className="xt-editor" onOpenChange={open => { if (!open) close() }}>
    <form onSubmit={event => {
      event.preventDefault()
      const title = String(new FormData(event.currentTarget).get("title")).trim()
      if (!title || !validPlanDate(date)) { setIssue("请填写标题，并选择一个有效日期。"); return }
      setIssue("")
      save({ id: value?.id ?? crypto.randomUUID(), title, date })
    }}>
      <label>标题<input name="title" defaultValue={value?.title} maxLength={80} required /></label>
      <div className="xt-date-control"><span>目标日期</span><PlanDatePicker value={date} today={today} change={setDate} disabled={false} countdown /></div>
      {(issue || error) && <p className="xt-error" role="alert">{issue || error}</p>}
      <button type="submit" className="xt-primary">保存倒数日</button>
      {value && <RemoveButton remove={() => remove(value.id)} />}
    </form>
  </Modal>
}

function WeeklyPlan({ todos, today, initialDate }: { todos: TodoController; today: string; initialDate?: string }) {
  const [weeks, setWeeks] = useState(() => initialDate ? dateDistance(monday(initialDate), monday(today)) / 7 : 0)
  const [editor, setEditor] = useState<{ item: TodoItem | null; id: string; date: string } | null>(null)
  const start = datePlus(monday(today), 7 * weeks)
  const dates = weekdays.map((name, index) => ({ name, date: datePlus(start, index) }))
  const add = (date: string) => setEditor({ item: null, id: crypto.randomUUID(), date })
  return <>
    <div className="xk-list-head"><h2>每周计划</h2>{weeks !== 0 && <button onClick={() => setWeeks(0)}>回到本周</button>}</div>
    <div className="xk-week-control">
      <button aria-label="计划上一周" onClick={() => setWeeks(weeks - 1)} disabled={start <= "2000-01-03"}><ChevronLeft size={18} /></button>
      <strong>{start.slice(5)} — {dates[6].date.slice(5)}</strong>
      <button aria-label="计划下一周" onClick={() => setWeeks(weeks + 1)} disabled={dates[6].date >= "2100-12-31"}><ChevronRight size={18} /></button>
    </div>
    <p className="xt-hint">这里显示有计划日期的待办。两处的新增、编辑和完成状态会同步。</p>
    {todos.error && <p className="xt-error" role="alert">{todos.error}<button onClick={() => void todos.refresh()}>重试</button></p>}
    {dates.map(({ name, date }) => {
      const items = todos.state?.items.filter(item => item.due_date === date) ?? []
      return <section key={date} className="xt-panel xk-plan-day">
        <div className="xk-day-head">
          <h3>{name} {date.slice(5)} {date === today && <small>今天</small>}</h3>
          <button aria-label={`添加 ${date} 的计划`} onClick={() => add(date)} disabled={!todos.available || todos.busy || !validPlanDate(date)}><Plus size={19} /></button>
        </div>
        {items.length === 0 && <p className="xk-no-plan">暂无计划</p>}
        {items.map(item => <TodoRow key={item.id} item={item} today={today} busy={todos.busy}
          toggle={() => void todos.complete(item, !item.completed)} edit={() => setEditor({ item, id: item.id, date })} />)}
      </section>
    })}
    {editor && <TodoEditor key={editor.id} todos={todos} value={editor.item} id={editor.id} initialDate={editor.date} close={() => setEditor(null)} />}
  </>
}
