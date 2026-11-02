import { useEffect, useRef, useState } from "react"
import { ArrowLeft, ArrowRight, Check, CheckCheck, Circle, ListChecks, Plus, Star, Trash2 } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import { useMobileBack } from "@/lib/mobile/back"
import { todoDateLabel, type TodoItem, type TodoDraft } from "@/lib/mobile/todos"
import type { TodoController } from "@/hooks/use-todos"
import { PlanDatePicker } from "./PlanDatePicker"
import { FinishBox } from "./BoxSurprises"
import { useContentTransition } from "@/hooks/use-content-transition"

function TodoRow({ item, today, busy, toggle, edit }: { item: TodoItem; today: string; busy: boolean; toggle: () => void; edit: () => void }) {
  const date = todoDateLabel(item, today)
  return <article className={`xt-row${item.completed ? " is-completed" : ""}`}>
    <button className="xt-check" role="checkbox" aria-checked={item.completed} aria-label={`${item.completed ? "重新打开" : "完成"}：${item.title}`} disabled={busy} onClick={toggle}>{item.completed ? <Check size={19} /> : <Circle size={21} />}</button>
    <button className="xt-row-content" onClick={edit}><strong>{item.title}{item.important && <Star size={12} fill="currentColor" aria-label="重要" />}</strong>{item.note && <p>{item.note}</p>}{date && <small className={!item.completed && item.due_date < today ? "is-overdue" : ""}>{date}</small>}</button>
  </article>
}

export function HomeTodos({ todos, open, add }: { todos: TodoController; open: () => void; add: () => void }) {
  const items = todos.state?.items.filter(i => !i.completed).slice(0, 3) ?? []
  return <section className="xt-home" aria-label="首页待办事项"><div className="xm-section-title"><h2>待办事项</h2><button onClick={open}>全部{todos.state?.counts.open ? ` · ${todos.state.counts.open}` : ""}<ArrowRight size={16} /></button></div>
    <div className="xt-panel">{todos.error && <p className="xt-error" role="alert">{todos.error}<button onClick={() => void todos.refresh()}>重试</button></p>}
      {items.map(i => <TodoRow key={i.id} item={i} today={todos.state?.today ?? ""} busy={todos.busy} toggle={() => void todos.complete(i, true)} edit={open} />)}
      {!items.length && !todos.error && <p className="xt-home-empty">{!todos.available ? "在手机 App 中记录你的待办。" : !todos.state ? "正在读取待办…" : todos.state.counts.completed ? "待办都已完成，给自己一点空闲。" : "记下一件想做的小事，一件件完成。"}</p>}
      <FinishBox state={todos.state} unavailable={!todos.available || !!todos.error || todos.busy} />
      <button className="xt-home-add" onClick={add} disabled={!todos.available}><Plus size={17} />添加待办</button>
    </div>
  </section>
}

export function TodoPage({ todos, close, createRequest }: { todos: TodoController; close: () => void; createRequest: number }) {
  const [filter, setFilter] = useState<"open" | "today" | "completed">("open"), [editor, setEditor] = useState<{ id: string; original: TodoItem | null } | null>(null)
  const panel = useRef<HTMLElement>(null)
  useContentTransition(panel, filter, ["open", "today", "completed"].indexOf(filter), !editor, true)
  const add = () => setEditor({ id: crypto.randomUUID(), original: null })
  const consumedRequest = useRef(0)
  useEffect(() => { if (createRequest > 0 && consumedRequest.current !== createRequest) { consumedRequest.current = createRequest; queueMicrotask(() => setEditor({ id: crypto.randomUUID(), original: null })) } }, [createRequest])
  useMobileBack(true, close, 10)
  const s = todos.state, items = s?.items.filter(i => filter === "completed" ? i.completed : !i.completed && (filter !== "today" || i.due_date === s.today)) ?? []
  return <section ref={panel} className="xt-page" aria-label="待办事项"><header className="xt-header"><button aria-label="返回上一页" onClick={close}><ArrowLeft size={21} /></button><div><h1>待办事项</h1><p>记下来，一件件完成。</p></div><button aria-label="新建待办" disabled={!todos.available || todos.busy} onClick={add}><Plus size={22} /></button></header>
    <div className="xt-overview"><ListChecks size={24} /><div><strong>{s?.counts.open ?? 0}<small>项待完成</small></strong><p>{s?.counts.overdue ? `${s.counts.overdue} 项已逾期 · ` : ""}按自己的节奏，完成眼前的小事。</p></div></div>
    <div className="xt-tabs" aria-label="待办筛选">{([{ id: "open", title: "待办", count: s?.counts.open }, { id: "today", title: "今天", count: s?.counts.today }, { id: "completed", title: "已完成", count: s?.counts.completed }] as const).map(f => <button key={f.id} aria-pressed={filter === f.id} onClick={() => setFilter(f.id)}>{f.title}<span>{f.count ?? 0}</span></button>)}</div>
    {todos.error && <p className="xt-error" role="alert">{todos.error}<button onClick={() => void todos.refresh()}>重新读取</button></p>}
    {!todos.available ? <p className="xt-hint">当前环境暂不支持本机待办，请使用最新版手机 App。</p> : !s ? <p className="xt-hint">正在读取本机待办…</p> : !items.length ? <div className="xt-empty"><CheckCheck size={36} /><h2>{filter === "completed" ? "还没有完成记录" : filter === "today" ? "今天没有计划事项" : "把想做的事，留在这里"}</h2><p>{filter === "completed" ? "勾选完成后，就能在这里找回。" : "事项保存在手机，离线也能查看。"}</p>{filter !== "completed" && <button className="xt-primary" onClick={add}><Plus size={16} />添加待办</button>}</div> : <div className="xt-panel xt-list">{items.map(i => <TodoRow key={i.id} item={i} today={s.today} busy={todos.busy} toggle={() => void todos.complete(i, !i.completed)} edit={() => setEditor({ id: i.id, original: i })} />)}</div>}
    <p className="xt-footnote">计划日期用于整理事项，不会自动发送提醒。</p>
    {editor && <TodoEditor key={editor.id} value={editor.original} id={editor.id} todos={todos} close={() => setEditor(null)} />}
  </section>
}

function TodoEditor({ value, id, todos, close }: { value: TodoItem | null; id: string; todos: TodoController; close: () => void }) {
  const [draft, setDraft] = useState<TodoDraft>({ title: value?.title ?? "", note: value?.note ?? "", due_date: value?.due_date ?? "", important: value?.important ?? false }), [deleting, setDeleting] = useState(false)
  const titleLength = [...draft.title.trim()].length, noteLength = [...draft.note].length, valid = titleLength > 0 && titleLength <= 120 && noteLength <= 2000 && (!draft.due_date || draft.due_date >= "2000-01-01" && draft.due_date <= "2100-12-31")
  return <Modal open title={value ? "编辑待办" : "添加待办"} description="只保存在手机，随时记下、随时查看。" className="xt-editor" onOpenChange={open => { if (!open && !todos.busy) close() }}><form onSubmit={e => { e.preventDefault(); const fields = new FormData(e.currentTarget); const liveDraft = { ...draft, title: String(fields.get("title") ?? ""), note: String(fields.get("note") ?? "") }; if (valid) void todos.save(id, value, liveDraft).then(ok => { if (ok) close() }) }}>
    <label>事项内容<input aria-label="事项内容" defaultValue={value?.title ?? ""} name="title" maxLength={240} placeholder="例如：整理本周笔记" onInput={e => { const title = e.currentTarget.value; setDraft(d => ({ ...d, title })) }} disabled={todos.busy} /></label><small className={titleLength > 120 ? "xt-error" : "xt-counter"}>{titleLength}/120</small>
    <label>备注（可选）<textarea aria-label="待办备注" defaultValue={value?.note ?? ""} name="note" maxLength={4000} rows={3} placeholder="补充一点细节" onInput={e => { const note = e.currentTarget.value; setDraft(d => ({ ...d, note })) }} disabled={todos.busy} /></label><small className={noteLength > 2000 ? "xt-error" : "xt-counter"}>{noteLength}/2000</small>
    <div className="xt-date-control"><span>计划日期（可选）</span><PlanDatePicker value={draft.due_date} today={todos.state?.today} change={due_date => setDraft(d => ({ ...d, due_date }))} disabled={todos.busy} /></div>
    <label className="xt-important"><span><Star size={16} />标记为重要</span><input type="checkbox" role="switch" checked={draft.important} onChange={e => setDraft({ ...draft, important: e.target.checked })} disabled={todos.busy} /></label>
    {todos.error && <p className="xt-error" role="alert">{todos.error}</p>}
    <button className="xt-primary" type="submit" disabled={!valid || todos.busy}>{todos.busy ? "正在保存…" : "保存待办"}</button>
    {value && !deleting && <button type="button" className="xt-delete" disabled={todos.busy} onClick={() => setDeleting(true)}><Trash2 size={15} />删除事项</button>}
    {deleting && value && <div className="xt-delete-confirm"><p>确定删除这条事项？删除后无法恢复。</p><button type="button" disabled={todos.busy} onClick={() => setDeleting(false)}>取消</button><button type="button" disabled={todos.busy} onClick={() => void todos.remove(value).then(ok => { if (ok) close() })}>确认删除</button></div>}
  </form></Modal>
}
