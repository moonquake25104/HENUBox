import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react"
import { useContentTransition } from "@/hooks/use-content-transition"
import { ArrowLeft, BookOpen, CalendarDays, Check, ChevronRight, CloudDownload, Database, ImagePlus, Info, Moon, Palette as PaletteIcon, RefreshCw, ShieldCheck, Smartphone, Sparkles } from "lucide-react"
import { useMobileBack } from "@/lib/mobile/back"
import { localWorkspaceBridge, type LocalBootstrap, type LocalConnection } from "@/lib/mobile/local-school"
import { PALETTES, type Palette, type useMobilePresentation } from "@/lib/mobile/presentation"
import type { UiStyle } from "@/lib/mobile/ui-style"
import type { SemesterSettings } from "@/lib/api/academic"
import { semesterLabel } from "@/components/xique/dates"
import { selectedCampus } from "@/lib/mobile/campus"
import "./me-settings.css"
import { CourseReminderSettings } from "./CourseReminderSettings"
import { GuideCacheSettings } from "./CampusGuidePage"
import { AboutBox } from "./BoxSurprises"
import { AboutLegal } from "./AboutLegal"
import { useTimetableDays } from "@/lib/mobile/timetable-days"
import "./course-reminders.css"

type View = "root" | "school" | "timetable" | "general" | "data" | "about"
type Props = { boot: LocalBootstrap; connection: LocalConnection | null; semester: string; courseCount: number; saved: string; todoCount: number; presentation: ReturnType<typeof useMobilePresentation>; style: UiStyle; setStyle: (style: UiStyle) => void; backgroundCustom: boolean; openBackground: () => void; openSchool: () => void; refresh: () => Promise<void> }
const titles: Record<View, string> = { root: "我的河小盒空间", school: "学校与账号", timetable: "课表与作息", general: "通用", data: "本机数据", about: "关于HENUBox" }
const descriptions: Record<View, string> = { root: "数据留在手机，生活掌握在自己手里。", school: "连接校园，也保留自己的安排。", timetable: "把学期与每一节课安排明白。", general: "选一个舒服的样子，按你的习惯来。", data: "已经保存的安排，随时都能查看。", about: "好好上课，也好好生活。" }
function Row({ icon, title, detail, value, action }: { icon: ReactNode; title: string; detail?: string; value?: string; action: () => void }) { return <button className="xp-row" aria-label={title} onClick={action}><span className="xp-row-icon">{icon}</span><span className="xp-row-copy"><strong>{title}</strong>{detail && <small>{detail}</small>}</span>{value && <em>{value}</em>}<ChevronRight size={16} /></button> }
function Group({ title, children }: { title: string; children: ReactNode }) { return <section className="xp-group"><h2>{title}</h2><div className="xp-card">{children}</div></section> }
function Fact({ name, value }: { name: string; value: ReactNode }) { return <div className="xp-fact"><span>{name}</span><strong>{value}</strong></div> }

export function MeSettings(p: Props) {
  const timetable = useTimetableDays()
  const [view, setView] = useState<View>("root"), [settings, setSettings] = useState<SemesterSettings | null>(null), [settingsError, setSettingsError] = useState(""), [rereading, setRereading] = useState(false), [readDone, setReadDone] = useState(false), [readError, setReadError] = useState("")
  const pref = p.presentation, name = PALETTES[pref.palette].name, mode = pref.mode === "system" ? "跟随系统" : pref.mode === "dark" ? "深色" : "浅色", material = p.style === "glass" ? "液态玻璃" : p.style === "android" ? "安卓原生" : "经典"
  const panel = useRef<HTMLElement>(null)
  useContentTransition(panel, view, view === "root" ? 0 : 1)
  const status = p.connection?.connection_state === "CONNECTED" ? "已连接" : p.connection?.connection_state === "AUTH_REQUIRED" ? "需要重新登录" : p.connection?.connection_state === "TEMPORARY_FAILURE" ? "连接暂不可用" : "未连接"
  useMobileBack(view !== "root", () => setView("root"), 5)
  useLayoutEffect(() => { window.scrollTo(0, 0); document.getElementById("xm-content")?.scrollTo(0, 0) }, [view])
  useEffect(() => {
    if (view !== "timetable") return
    let live = true; setSettings(null); setSettingsError("")
    void localWorkspaceBridge.getSettings({ workspace_id: p.boot.workspace_id, semester: p.semester }).then(v => { if (live) setSettings(v.settings) }).catch(() => { if (live) setSettingsError("学期设置暂时无法读取，请稍后重试。") })
    return () => { live = false }
  }, [view, p.boot.workspace_id, p.boot.revision, p.semester])
  const reload = async () => { if (rereading) return; setRereading(true); setReadDone(false); setReadError(""); try { await p.refresh(); setReadDone(true) } catch { setReadError("本机课表暂时无法重新读取，已显示的安排仍保留。") } finally { setRereading(false) } }
  return <section ref={panel} className={`xm-settings xp-${view}`} aria-label="我的设置">
    <header className={view === "root" ? "xp-profile" : "xp-header"}>{view === "root" ? <span className="xp-brand"><BookOpen size={26} /></span> : <button aria-label="返回我的" onClick={() => setView("root")}><ArrowLeft size={21} /></button>}<div><h1>{titles[view]}</h1><p>{descriptions[view]}</p></div></header>
    {view === "root" && <>
      <button className="xp-account" aria-label="查看学校与账号" onClick={() => setView("school")}><span className="xp-account-icon"><CloudDownload size={23} /></span><span><strong>河南大学</strong><small>{status} · {semesterLabel(p.semester)}</small></span><ChevronRight size={17} /></button>
      <Group title="学习与课表"><Row icon={<ShieldCheck size={19} />} title="学校与账号" detail="本人授权 · 学校连接与更新" value={status} action={() => setView("school")} /><Row icon={<CalendarDays size={19} />} title="课表与作息" detail="学期、校区与上课时间" value={semesterLabel(p.semester)} action={() => setView("timetable")} /></Group>
      <Group title="个人偏好"><Row icon={<Sparkles size={19} />} title="通用" detail="界面风格 · 浅深色 · 配色 · 首页背景" value={material} action={() => setView("general")} /><Row icon={<Database size={19} />} title="本机数据" detail="保存状态与重新读取" value={`${p.courseCount} 个安排`} action={() => setView("data")} /></Group>
      <Group title="关于"><Row icon={<Info size={19} />} title="关于HENUBox" detail="版本与使用说明" value="0.1.0" action={() => setView("about")} /></Group>
      <p className="xp-footer">好好上课，也好好生活。</p>
    </>}
    {view === "school" && <>
      <div className="xp-account xp-account-static"><span className="xp-account-icon"><ShieldCheck size={25} /></span><span><strong>河南大学</strong><small>{status}</small></span><span className={`xp-status ${status === "已连接" ? "is-connected" : ""}`}>{status}</span></div>
      <Group title="学校连接"><Row icon={<CloudDownload size={19} />} title="学校连接与更新" detail={status === "已连接" ? "检查学校课表更新，核对后保存到手机" : "登录学校账号，选择校区后导入课表"} action={p.openSchool} /></Group>
      <div className="xp-note"><ShieldCheck size={18} /><p>由你本人完成学校登录。课表与空闲教室共用学校连接，已经保存的课表离线也能查看。</p></div>
    </>}
    {view === "timetable" && <>
      <Group title="课表显示"><div className="xp-option"><h3><CalendarDays size={18} />每周显示天数</h3><div className="xp-segment" role="group" aria-label="每周显示天数">{([5, 7] as const).map(days => <button key={days} aria-pressed={timetable.days === days} onClick={() => timetable.setDays(days)}>{days === 5 ? "五天" : "七天"}</button>)}</div><p>五天显示周一至周五；周末课程仍保留，可切换七天或列表查看。</p></div></Group>
      <Group title="当前学期"><Fact name="学期" value={semesterLabel(p.semester)} /><Fact name="校区" value={settings ? settings.campus || selectedCampus(settings) || "未设置" : "读取中…"} /><Fact name="教学周" value="1–18 周" /><Fact name="第一周周一" value={settings?.week_anchor.start_date || "尚未读取"} /></Group>
      {settingsError && <p className="xp-error" role="alert">{settingsError}</p>}
      <Group title="上课提醒"><CourseReminderSettings /></Group>
      <Group title="作息时间">{settings?.periods?.length ? <div className="xp-periods">{settings.periods.map(period => <div key={period.index}><span>第 {period.index} 节</span><strong>{period.start_time?.slice(0, 5) || "待设置"} <i>—</i> {period.end_time?.slice(0, 5) || "待设置"}</strong></div>)}</div> : <p className="xp-copy">{settingsError ? "请打开学期设置查看或重新读取。" : "正在读取已保存的作息…"}</p>}<Row icon={<CalendarDays size={19} />} title="调整学期与校区" detail="打开现有学校连接与更新，核对学期及作息" action={p.openSchool} /></Group>
    </>}
    {view === "general" && <>
      <Group title="外观"><div className="xp-option"><h3><Sparkles size={18} />界面风格</h3><div className="xp-segment" role="group" aria-label="界面风格">{([['classic', '经典'], ['glass', '液态玻璃'], ['android', '安卓原生']] as const).map(([id, label]) => <button key={id} aria-pressed={p.style === id} onClick={() => p.setStyle(id)}>{label}</button>)}</div><p>相同的布局，选择喜欢的质感。</p></div><div className="xp-option"><h3><Moon size={18} />浅深色</h3><div className="xp-segment" role="group" aria-label="浅深色">{([['system', '跟随系统'], ['light', '浅色'], ['dark', '深色']] as const).map(([id, label]) => <button key={id} aria-pressed={pref.mode === id} onClick={() => pref.setMode(id)}>{label}</button>)}</div></div></Group>
      <Group title="配色方案"><div className="xp-palette-grid" role="group" aria-label="配色方案">{(Object.keys(PALETTES) as Palette[]).map(id => <button key={id} aria-label={`配色：${PALETTES[id].name}`} aria-pressed={pref.palette === id} onClick={() => pref.setPalette(id)}><span className="xp-swatches" aria-hidden="true">{PALETTES[id].swatches.map(color => <i key={color} style={{ background: color }} />)}</span><strong>{PALETTES[id].name}{pref.palette === id && <Check size={14} />}</strong><small>{PALETTES[id].description}</small></button>)}</div><div className="xp-palette-preview"><span className="xp-preview-chip"><PaletteIcon size={18} /></span><span><strong>{name}</strong><small aria-live="polite">{material} · {mode}</small></span><i /><i /></div><p className="xp-caption">每套配色均包含浅色与深色，三种界面风格都可使用。</p></Group>
      <Group title="首页背景"><Row icon={<ImagePlus size={19} />} title="首页背景" detail="本机图片、GIF 与静音短视频" value={p.backgroundCustom ? "已自定义" : "默认插画"} action={p.openBackground} /></Group>
    </>}
    {view === "data" && <>
      <Group title="已经保存在手机"><Fact name="本机课表" value={`${p.courseCount} 个安排`} /><Fact name="待办事项" value={`${p.todoCount} 条`} /><Fact name="最近保存课表" value={p.saved} /></Group>
      <Group title="公开资料缓存"><GuideCacheSettings /></Group>
      <Group title="读取本机数据"><button className="xp-row" aria-label="重新读取本机课表" disabled={rereading} onClick={() => void reload()}><span className="xp-row-icon"><RefreshCw size={19} className={rereading ? "animate-spin" : undefined} /></span><span className="xp-row-copy"><strong>{rereading ? "正在读取…" : "重新读取本机课表"}</strong><small>从手机读取已保存的课表</small></span><ChevronRight size={16} /></button></Group>
      {readDone && <p className="xp-caption" role="status">本机课表已重新读取。</p>}{readError && <p className="xp-error" role="alert">{readError}</p>}<div className="xp-note"><Smartphone size={18} /><p>离线时也能查看已保存的课表、待办与专注记录。学校的最新课表需要在联网后检查更新。</p></div>
    </>}
    {view === "about" && <><div className="xp-about-brand"><span className="xp-brand"><BookOpen size={30} /></span><AboutBox /><p>河小盒 · 好好上课，也好好生活。</p><small>版本 0.1.0</small></div><AboutLegal /></>}
  </section>
}
