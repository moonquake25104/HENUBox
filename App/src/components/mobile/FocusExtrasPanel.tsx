import { useEffect, useState } from "react"
import { Capacitor, registerPlugin } from "@capacitor/core"
import { Headphones, ShieldCheck, Upload } from "lucide-react"
import { Modal } from "@/components/shared/modal"

type Extras = { sound: string; volume: number; custom_name: string; custom_available: boolean; audio_status: string; dnd_choice: string; dnd_access: boolean; dnd_active: boolean }
interface Bridge {
  getState(): Promise<Extras>
  saveAudio(value: { sound: string; volume: number }): Promise<Extras>
  importAudio(): Promise<Extras>
  dndChoice(value: { choice: "enabled" | "declined" }): Promise<Extras>
}
const bridge = registerPlugin<Bridge>("XiqueFocusExtras")
const sounds = [{ id: "off", name: "关闭" }, { id: "rain", name: "窗边雨声" }, { id: "waves", name: "海岸浪声" }, { id: "pink", name: "粉噪音" }, { id: "brown", name: "棕噪音" }]
export function FocusExtrasPanel() {
  const [s, set] = useState<Extras | null>(null), [busy, setBusy] = useState(false), [error, setError] = useState(""), [ask, setAsk] = useState(false)
  useEffect(() => {
    if (!Capacitor.isPluginAvailable("XiqueFocusExtras")) return
    let live = true
    const load = () => { if (!document.hidden) void bridge.getState().then(v => { if (live) set(v) }).catch(() => { if (live) setError("专注音效设置暂时无法读取。") }) }
    load(); const timer = window.setInterval(load, 2000)
    return () => { live = false; window.clearInterval(timer) }
  }, [])
  const act = async (fn: () => Promise<Extras>) => {
    if (busy) return
    setBusy(true); setError("")
    try { set(await fn()) } catch (e) {
      const code = (e as { code?: string }).code
      if (code !== "CANCELLED") setError(code === "AUDIO_LIMIT" ? "音频请控制在 20 MB、1 秒至 30 分钟以内。" : code === "AUDIO_FORMAT" ? "无法读取这段音频，请换成 MP3、M4A、OGG 或 WAV。" : "操作未完成，原设置仍保留，请重试。")
    } finally { setBusy(false) }
  }
  if (!s) return null
  return <>
    <section className="xf-panel xf-extras"><h2><Headphones size={18} />专注背景音</h2><p className="xf-muted">随专注开始，暂停、休息或结束时停止。来电、其他音频或拔下耳机后静音，暂停再继续可恢复。</p>
      <div className="xf-sound-choices">{sounds.map(sound => <button key={sound.id} aria-pressed={s.sound === sound.id} disabled={busy} onClick={() => void act(() => bridge.saveAudio({ sound: sound.id, volume: s.volume }))}>{sound.name}</button>)}{s.custom_available && <button aria-pressed={s.sound === "custom"} disabled={busy} onClick={() => void act(() => bridge.saveAudio({ sound: "custom", volume: s.volume }))}>我的音频</button>}</div>
      <label className="xf-audio-volume">音量 <select aria-label="背景音音量" value={s.volume} disabled={busy} onChange={e => void act(() => bridge.saveAudio({ sound: s.sound, volume: Number(e.target.value) }))}>{[0, 15, 25, 35, 50, 65, 80, 100].map(v => <option key={v} value={v}>{v}%</option>)}</select></label>
      <button className="xf-secondary" disabled={busy} onClick={() => void act(() => bridge.importAudio())}><Upload size={16} />从手机导入音频</button>
      {s.custom_name && <p className="xf-muted">已导入：{s.custom_name} · 再次导入将替换</p>}
      <p className="xf-muted">音频只存本机 · 20 MB / 30 分钟以内。内置自然声为 CC0 授权短循环，粉 / 棕噪音为合成音。</p>
      <p role="status" className="xf-audio-status">{({ playing: "正在播放", loading: "准备播放…", interrupted: "背景音已暂停，暂停再继续计时可恢复", error: "背景音无法播放，请重新选择；计时正常", off: "未播放" } as Record<string, string>)[s.audio_status]}</p>
    </section>
    <section className="xf-panel xf-extras"><h2><ShieldCheck size={18} />系统勿扰</h2><p className="xf-muted">{s.dnd_active ? "专注勿扰已开启，暂停或结束时撤销本应用规则。" : s.dnd_choice === "enabled" ? s.dnd_access ? "已授权。专注时使用自己的勿扰规则；系统关闭或删除的规则不会强行恢复。" : "尚未获得系统授权，目前不会开启勿扰。" : s.dnd_choice === "declined" ? "按你的选择，未启用系统勿扰。" : "默认关闭，由你选择是否授权。"}</p>
      <button className="xf-secondary" disabled={busy} onClick={() => s.dnd_choice === "enabled" && s.dnd_access ? void act(() => bridge.dndChoice({ choice: "declined" })) : setAsk(true)}>{s.dnd_choice === "enabled" && s.dnd_access ? "关闭专注勿扰" : "设置系统勿扰"}</button>
    </section>
    {error && <p role="alert" className="xm-notice">{error}</p>}
    {ask && <Modal open title="专注时开启系统勿扰？" onOpenChange={setAsk}><p className="xf-muted">同意后将打开系统授权页，由你决定是否允许。专注时启用本应用的勿扰规则，保留闹钟、重要联系人和重复来电；暂停、休息或结束后撤销自己的规则。你的其他勿扰设置仍由系统管理。未授权不会启用。</p><div className="xf-controls"><button className="xf-secondary" onClick={() => { setAsk(false); void act(() => bridge.dndChoice({ choice: "declined" })) }}>不启用</button><button className="xf-primary" onClick={() => { setAsk(false); void act(() => bridge.dndChoice({ choice: "enabled" })) }}>同意，前往系统授权</button></div></Modal>}
  </>
}
