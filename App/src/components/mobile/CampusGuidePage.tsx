import { useCallback, useEffect, useRef, useState, type ReactNode } from "react"
import { ArrowLeft, ArrowUpRight, BadgeInfo, BookOpen, Bus, CalendarDays, Check, ChevronRight, Download, Expand, ImageOff, LoaderCircle, Map, Music2, Phone, RefreshCw, ShieldCheck, Trash2, Users, View } from "lucide-react"
import { Capacitor, registerPlugin, type PluginListenerHandle } from "@capacitor/core"
import { useMobileBack } from "@/lib/mobile/back"
import { useContentTransition } from "@/hooks/use-content-transition"
import { localWorkspaceBridge } from "@/lib/mobile/local-school"
import { selectedCampus } from "@/lib/mobile/campus"
import { GUIDE_PAGES, clearGuideCache, dialGuidePhone, guideAsset, guideCacheInfo, loadGuide, openGuideSource, saveVisitGuide, viewGuideImage, type DocumentKind, type GuideDocument, type GuideKind } from "@/lib/mobile/guide"
import type { GuideImage } from "@/lib/mobile/guide-parser"
import "./campus-guide.css"

const icons: Record<GuideKind, ReactNode> = { calendar: <CalendarDays />, maps: <Map />, phones: <Phone />, shuttle: <Bus />, visits: <Users />, vr: <View />, emblem: <ShieldCheck />, anthem: <Music2 /> }
const groups: { title: string; items: GuideKind[] }[] = [{ title: "常用查询", items: ["calendar", "maps", "phones"] }, { title: "出行与办事", items: ["shuttle", "visits"] }, { title: "认识河大", items: ["vr", "emblem", "anthem"] }]
const time = (n: number) => new Date(n).toLocaleString("zh-CN", { timeZone: "Asia/Shanghai", month: "numeric", day: "numeric", hour: "2-digit", minute: "2-digit" })
const message = (e: unknown) => (e as { code?: string })?.code === "PERMISSION_DENIED" ? "没有保存到相册。你可以再次点击并选择允许。" : "暂时无法完成，请检查网络后重试，也可以查看官网。"

export function CampusGuidePage({ close, workspaceId, semester }: { close: () => void; workspaceId: string; semester: string }) {
  const [kind, setKind] = useState<DocumentKind | null>(null), [campus, setCampus] = useState("郑州"), [error, setError] = useState(""), [opening, setOpening] = useState(false)
  const content = useRef<HTMLElement>(null)
  useContentTransition(content, kind ?? "overview", kind ? 1 : 0, true, true)
  const back = () => kind ? setKind(null) : close()
  useMobileBack(true, back, 15)
  useEffect(() => { let live = true; void localWorkspaceBridge.getSettings({ workspace_id: workspaceId, semester }).then(v => { if (live) setCampus(v.settings.campus || selectedCampus(v.settings) || "郑州") }).catch(() => {}); return () => { live = false } }, [workspaceId, semester])
  async function choose(id: GuideKind) { setError(""); if (id !== "vr") { setKind(id); return } setOpening(true); try { await openGuideSource("vr") } catch (e) { setError(message(e)) } finally { setOpening(false) } }
  return <section className="xg-page" aria-label="校园指南"><header className="xg-header"><button aria-label={kind ? "返回校园指南" : "返回Campus"} onClick={back}><ArrowLeft size={22} /></button><div><h1>{kind ? GUIDE_PAGES[kind].title : "校园指南"}</h1><p>{kind ? GUIDE_PAGES[kind].detail : "查信息，逛校园，认识河大。"}</p></div><span className="xg-header-mark"><BookOpen size={22} /></span></header>
    <main ref={content} className="xg-content">{kind ? <GuideDetail key={kind} kind={kind} campus={campus} semester={semester} /> : <>
      <div className="xg-welcome"><div><small>HENUBox · 河小盒</small><h2>校园的大小事，<br />从这里找到。</h2><p>学校公开信息，按你的习惯整理。</p></div><Map size={68} strokeWidth={1.1} /></div>
      {groups.map(g => <section className="xg-group" key={g.title}><h2>{g.title}</h2><div className="xg-list">{g.items.map(id => <button key={id} className={`xg-entry xg-entry-${id}`} disabled={opening} onClick={() => void choose(id)}><span className="xg-entry-icon">{icons[id]}</span><span><strong>{GUIDE_PAGES[id].title}</strong><small>{GUIDE_PAGES[id].detail}</small></span>{id === "vr" ? opening ? <LoaderCircle size={17} className="animate-spin" /> : <ArrowUpRight size={19} /> : <ChevronRight size={19} />}</button>)}</div></section>)}
      {error && <p role="alert" className="xg-error">{error}</p>}<p className="xg-footnote">公开资料来自学校官网。图文按需保存，全景点开后联网加载。</p>
    </>}</main></section>
}

function GuideDetail({ kind, campus, semester }: { kind: DocumentKind; campus: string; semester: string }) {
  const [result, setResult] = useState<{ document: GuideDocument; stale: boolean; cache: boolean } | null>(null), [busy, setBusy] = useState(true), [error, setError] = useState(""), [notice, setNotice] = useState(""), [refreshToken, setRefreshToken] = useState(0)
  const serial = useRef(0), live = useRef(true)
  const run = useCallback(async (refresh: boolean) => { const request = ++serial.current; setBusy(true); setError(""); setNotice(""); try { const value = await loadGuide(kind, refresh); if (live.current && request === serial.current) { setResult(value); if (refresh && !value.stale) setRefreshToken(n => n + 1) } } catch { if (live.current && request === serial.current) setError("这份资料暂时无法读取。请联网重试，或查看学校官网。") } finally { if (live.current && request === serial.current) setBusy(false) } }, [kind])
  useEffect(() => { live.current = true; let mounted = true; queueMicrotask(() => { if (mounted) void run(false) }); return () => { mounted = false; live.current = false } }, [run])
  const source = async () => { try { await openGuideSource(kind) } catch (e) { setNotice(message(e)) } }
  const d = result?.document
  return <>
    <div className="xg-source"><span><BadgeInfo size={14} />河南大学官网</span><button onClick={() => void source()}>查看原网页<ArrowUpRight size={14} /></button></div>
    <div className="xg-update"><span role="status">{busy ? "正在读取官方资料…" : d ? `${result.stale ? "更新失败 · 显示旧缓存" : result.cache ? "本机缓存" : "已获取"} · ${time(d.fetchedAt)}` : "暂未获取资料"}</span><button disabled={busy} aria-label="检查指南更新" onClick={() => void run(true)}><RefreshCw size={15} className={busy ? "animate-spin" : undefined} />更新</button></div>
    {result?.stale && <p className="xg-warning">当前显示上次成功获取的内容，可能已有调整。联网后可再次更新。</p>}
    {error && <div className="xg-empty" role="alert"><ImageOff size={32} /><p>{error}</p><button className="xg-primary" onClick={() => void run(true)}>重新读取</button></div>}
    {notice && <p className="xg-warning" role="status">{notice}</p>}
    {d && <>
      {kind === "calendar" && <p className="xg-note">按官方校历完整展示，含复习考试周；不会修改你的课表周次或作息。</p>}
      {kind === "maps" && <p className="xg-note">官方校区示意图。切换地图不影响课表校区；点击图片可分块放大查看。</p>}
      {kind === "shuttle" && <p className="xg-note">此处为官方时刻表查询。请留意工作日、周末、假期和冬夏时间，临时调整以学校通知为准。</p>}
      {kind === "visits" && <div className="xg-note"><strong>在微信办理</strong><p>先查看官方流程。可将流程图保存到相册，再用微信“扫一扫 → 相册”识别二维码，由你本人在学校平台完成申请。</p><button className="xg-primary" onClick={() => { setNotice("正在保存官方流程图…"); void saveVisitGuide(d.images[0].url).then(() => { if (live.current) setNotice("已保存到相册，请在微信扫一扫中选择这张图片。") }).catch(e => { if (live.current) setNotice(message(e)) }) }}><Download size={16} />保存官方流程图</button></div>}
      {kind === "phones" && <PhoneList document={d} report={setNotice} />}
      {kind === "anthem" && d.audio && <AnthemPlayer url={d.audio} />}
      {!!d.images.length && <GuideGallery images={d.images} defaultLabel={kind === "maps" ? campus : kind === "calendar" && semester === "2026S2" ? "秋季" : ""} refreshToken={refreshToken} />}
      {d.sections.map((s, i) => <section className="xg-article" key={i}><h2>{s.title}</h2>{s.images.map((im, j) => <GuidePicture key={im.url} image={{ ...im, title: s.images.length > 1 ? `${s.title} · 图 ${j + 1}` : s.title }} refreshToken={refreshToken} />)}{s.paragraphs.map((p, j) => <p key={j}>{p}</p>)}</section>)}
      <p className="xg-footnote">来源：{GUIDE_PAGES[kind].title} · 河南大学<br />上方时间为本机获取时间。学校原图保留原色。</p>
    </>}
  </>
}

function GuideGallery({ images, defaultLabel, refreshToken }: { images: GuideImage[]; defaultLabel: string; refreshToken: number }) {
  const [selectedTitle, setSelectedTitle] = useState("")
  const preferred = images.findIndex(im => selectedTitle ? im.title === selectedTitle : defaultLabel && im.title.includes(defaultLabel)), index = Math.max(0, preferred), chosen = images[index]
  return <div className="xg-gallery"><label className="xg-select"><span>查看内容</span><select aria-label="指南内容选择" value={index} onChange={e => setSelectedTitle(images[Number(e.target.value)].title)}>{images.map((im, i) => <option key={im.url} value={i}>{im.title}</option>)}</select></label><GuidePicture key={chosen.url} image={chosen} refreshToken={refreshToken} /></div>
}

function GuidePicture({ image, refreshToken }: { image: GuideImage; refreshToken: number }) {
  const ref = useRef<HTMLDivElement>(null), [visible, setVisible] = useState(false), [asset, setAsset] = useState<{ uri: string; stale: boolean } | null>(null), [error, setError] = useState(""), [retry, setRetry] = useState(0), [opening, setOpening] = useState(false)
  useEffect(() => { const target = ref.current; if (!target) return; const observer = new IntersectionObserver(es => { if (es.some(e => e.isIntersecting)) setVisible(true) }, { rootMargin: "180px" }); observer.observe(target); return () => observer.disconnect() }, [])
  useEffect(() => { if (!visible) return; let live = true; void guideAsset(image.url, refreshToken > 0).then(a => { if (live) { setAsset(a); setError("") } }).catch(() => { if (live) setError("图片暂时无法读取，请重试或查看官网。") }); return () => { live = false } }, [image.url, visible, refreshToken, retry])
  const open = async () => { setOpening(true); try { await viewGuideImage(image.url, image.title) } catch { setError("暂时无法打开原图，请重试。") } finally { setOpening(false) } }
  return <div className="xg-picture" ref={ref}><div className="xg-picture-label"><strong>{image.title}</strong><span><Expand size={14} />点击放大</span></div>{asset ? <button className="xg-image-button" aria-label={`放大查看${image.title}`} disabled={opening} onClick={() => void open()}><img src={asset.uri} alt={image.title} onError={() => setError("本机图片缓存已失效，请重新读取。")} /></button> : !error ? <div className="xg-image-placeholder"><LoaderCircle size={22} className="animate-spin" /><span>按需加载图片</span></div> : null}{asset?.stale && <p className="xg-caption">图片更新失败，正在显示已有缓存。</p>}{error && <p className="xg-image-error" role="alert">{error}<button onClick={() => { setError(""); setRetry(n => n + 1) }}>重新读取</button></p>}</div>
}

function PhoneList({ document: d, report }: { document: GuideDocument; report: (s: string) => void }) {
  const [query, setQuery] = useState("")
  const phones = d.phones.filter(p => `${p.department} ${p.internal} ${p.external}`.toLowerCase().includes(query.trim().toLowerCase()))
  return <><label className="xg-search"><Phone size={17} /><input aria-label="搜索校内电话" placeholder="搜索部门或号码" value={query} onChange={e => setQuery(e.target.value)} /></label><p className="xg-count">{phones.length} 个部门 · 内线与外线分别显示</p><div className="xg-phones">{phones.map((p, i) => <article className="xg-phone" key={`${p.department}-${i}`}><h2>{p.department}</h2><div><span>内线</span><strong>{p.internal || "未公布"}</strong></div><div><span>外线</span><strong>{p.external || "未公布"}</strong></div><footer><button onClick={() => { void navigator.clipboard.writeText(`${p.department}\n内线：${p.internal || "未公布"}\n外线：${p.external || "未公布"}`).then(() => report("已复制部门与号码。"), () => report("暂时无法复制，可以长按号码选取。")) }}>复制</button>{p.dial.map(n => <button key={n} onClick={() => void dialGuidePhone(n).catch(() => report("暂时无法打开拨号界面，请复制号码。"))}><Phone size={13} />拨号 {n}</button>)}</footer></article>)}{!phones.length && <p className="xg-note">没有匹配的部门或号码，换个关键词试试。</p>}</div></>
}

function AnthemPlayer({ url }: { url: string }) {
  const [src, setSrc] = useState(""), [busy, setBusy] = useState(false), [error, setError] = useState("")
  const audio = useRef<HTMLAudioElement>(null), alive = useRef(true)
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  useEffect(() => {
    const stop = () => { audio.current?.pause() }, hidden = () => { if (document.hidden) stop() }; let listener: PluginListenerHandle | undefined, live = true
    document.addEventListener("visibilitychange", hidden)
    if (Capacitor.isPluginAvailable("App")) { const app = registerPlugin<{ addListener(n: "appStateChange", cb: (v: { isActive: boolean }) => void): Promise<PluginListenerHandle> }>("App"); void app.addListener("appStateChange", v => { if (!v.isActive) stop() }).then(h => { if (live) listener = h; else void h.remove() }).catch(() => {}) }
    const element = audio.current
    return () => { live = false; document.removeEventListener("visibilitychange", hidden); void listener?.remove(); element?.pause(); if (element) { element.removeAttribute("src"); element.load() } }
  }, [src])
  async function load() { setBusy(true); setError(""); try { const a = await guideAsset(url); if (alive.current) setSrc(a.uri) } catch { if (alive.current) setError("校歌音频暂时无法加载，请联网后重试。") } finally { if (alive.current) setBusy(false) } }
  return <section className="xg-music"><span className="xg-music-icon"><Music2 size={30} /></span><div><h2>河南大学校歌</h2><p>官方音频 · 点按播放</p></div>{!src && <button className="xg-primary" disabled={busy} onClick={() => void load()}>{busy ? "正在加载…" : "加载校歌"}</button>}{src && <audio ref={audio} controls preload="none" src={src} aria-label="河南大学校歌播放器" onError={() => setError("音频暂时无法播放，请重新加载或查看官网。")} />}{error && <p className="xg-error" role="alert">{error}<button onClick={() => void load()}>重试</button></p>}<small>音频按需保存在本机，离开本页或进入后台时暂停。</small></section>
}

export function GuideCacheSettings() {
  const [info, setInfo] = useState<{ bytes: number; documents: number } | null>(null), [busy, setBusy] = useState(false), [note, setNote] = useState("")
  useEffect(() => { let live = true; void guideCacheInfo().then(v => { if (live) setInfo(v) }).catch(() => {}); return () => { live = false } }, [])
  return <section className="xg-cache"><h3><Map size={17} />校园指南缓存</h3><p>{info ? `${info.documents} 份图文资料 · ${(info.bytes / 1024 / 1024).toFixed(1)} MB 图片与音频` : "查看本机保存的公开资料"}</p><button disabled={busy} onClick={() => { setBusy(true); void clearGuideCache().then(() => { setInfo({ bytes: 0, documents: 0 }); setNote("指南缓存已清理。课表、待办和专注记录保留。") }).catch(() => setNote("缓存暂时无法清理，请稍后重试。")).finally(() => setBusy(false)) }}><Trash2 size={15} />{busy ? "正在清理…" : "清理指南缓存"}</button>{note && <p role="status"><Check size={14} />{note}</p>}</section>
}
