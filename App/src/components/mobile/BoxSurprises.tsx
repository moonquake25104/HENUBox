import { useRef, useState, type RefObject } from "react"
import { Modal } from "@/components/shared/modal"
import type { TodoState } from "@/lib/mobile/todos"
import "./box-surprises.css"

const notes = [
  "把日子一页页翻过去，答案会慢慢出现。",
  "今天的小小进步，也值得被好好收藏。",
  "不必赶上谁，走在自己的路上就很好。",
  "校园里的风，会替你带走一点烦恼。",
  "记得抬头看看天，也记得照顾自己。",
  "愿你有认真上课的日子，也有自在发呆的下午。",
  "把好奇心留着，明天还有新的风景。",
  "一件小事慢慢做，也能让生活亮一点。",
]
const blessings = [
  "今天的努力已经装进盒子里。接下来的时间，留给自己。",
  "做完了就歇一歇吧。休息，也是认真生活的一部分。",
  "谢谢今天认真生活的你。去吃顿喜欢的饭，好好收工。",
  "今日的小事已妥善安放，愿你今晚睡得安稳。",
]
// Match TodoStore.today(), even if the phone uses a different system time zone.
const todoDay = new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai", year: "numeric", month: "2-digit", day: "2-digit" })
function usePaper(pool: readonly string[]) {
  const last = useRef(-1), [paper, setPaper] = useState<string | null>(null)
  const opener = useRef<HTMLElement | null>(null)
  const open = () => {
    opener.current = document.activeElement instanceof HTMLElement ? document.activeElement : null
    const value = crypto.getRandomValues(new Uint32Array(1))[0]
    const next = last.current < 0 ? value % pool.length : (last.current + 1 + value % (pool.length - 1)) % pool.length
    last.current = next; setPaper(pool[next])
  }
  return { paper, open, opener, close: () => setPaper(null) }
}

/** Small vector only: the lid moves once when a paper is opened. */
function LittleBox({ opened = false }: { opened?: boolean }) {
  return <svg className={`xb-box${opened ? " is-open" : ""}`} viewBox="-16 -16 152 128" fill="none" aria-hidden="true">
    <ellipse cx="60" cy="98" rx="39" ry="6" fill="currentColor" opacity=".08" />
    <path d="m24 48 36-15 36 15v39L60 102 24 87z" fill="var(--xb-fill, #c9dfcf)" stroke="currentColor" strokeWidth="2.5" strokeLinejoin="round" />
    <path d="m24 48 36 15 36-15M60 63v39" stroke="currentColor" strokeWidth="2.5" opacity=".65" />
    <g className="xb-lid"><path d="m18 39 42-17 42 17v12L60 68 18 51z" fill="var(--xb-lid, #e1eadc)" stroke="currentColor" strokeWidth="2.5" strokeLinejoin="round" /><path d="m18 39 42 17 42-17M60 56v12" stroke="currentColor" strokeWidth="2.5" opacity=".6" /></g>
    {opened && <g className="xb-spark" fill="currentColor" opacity=".6"><path d="m-5 19 2 5 5 2-5 2-2 5-2-5-5-2 5-2z" /><path d="m124 5 2 5 5 2-5 2-2 5-2-5-5-2 5-2z" /></g>}
  </svg>
}
function Paper({ text, title, description, close, opener }: { text: string | null; title: string; description: string; close: () => void; opener: RefObject<HTMLElement | null> }) {
  return <Modal open={text !== null} title={title} description={description} onOpenChange={open => { if (!open) close() }} returnFocusRef={opener} className="xb-paper">
    {text !== null && <><div className="xb-open-art"><LittleBox opened /></div><div className="xb-note"><p>{text}</p><span>河小盒 · 给你的小纸条</span></div><button className="xb-done" onClick={close}>收好这份心意</button></>}
  </Modal>
}

export function AboutBox() {
  const taps = useRef({ count: 0, at: 0 }), paper = usePaper(notes)
  const tap = () => {
    const now = performance.now(), previous = taps.current
    taps.current = { count: now - previous.at < 1800 ? previous.count + 1 : 1, at: now }
    if (taps.current.count >= 5) { taps.current.count = 0; paper.open() }
  }
  return <><h2 className="xb-brand-heading"><button className="xb-brand-button" onClick={tap}>HENUBox</button></h2><Paper text={paper.paper} title="开盒小纸条" description="你发现了藏在名字里的小盒子。" close={paper.close} opener={paper.opener} /></>
}

function canFinishToday(state: TodoState | null) {
  if (!state || !/^\d{4}-\d{2}-\d{2}$/.test(state.today)) return false
  const planned = state.items.filter(item => item.due_date === state.today)
  if (planned.some(item => !item.completed)) return false
  const completedToday = state.items.some(item => {
    if (!item.completed || !item.completed_at || !Number.isFinite(item.completed_at)) return false
    const date = new Date(item.completed_at)
    return !Number.isNaN(date.getTime()) && todoDay.format(date) === state.today
  })
  return planned.length > 0 && planned.every(item => item.completed) || completedToday
}

export function FinishBox({ state, unavailable }: { state: TodoState | null; unavailable: boolean }) {
  return !unavailable && canFinishToday(state) ? <FinishPaper /> : null
}
function FinishPaper() {
  const paper = usePaper(blessings)
  return <div className="xb-finish"><button className="xb-finish-button" onClick={paper.open}><LittleBox /><span><strong>收工小盒子</strong><small>今天的计划已完成，打开看看</small></span><span aria-hidden="true">↗</span></button><Paper text={paper.paper} title="收工时刻" description="今天的小事，已经好好完成。" close={paper.close} opener={paper.opener} /></div>
}

