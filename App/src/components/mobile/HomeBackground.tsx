import { useRef } from "react"
import { ImagePlus, RotateCcw } from "lucide-react"
import { Modal } from "@/components/shared/modal"
import type { useHomeBackground } from "@/hooks/use-home-background"

export function HomeBackgroundSettings({ value, close }: { value: ReturnType<typeof useHomeBackground>; close: () => void }) {
  const input = useRef<HTMLInputElement>(null)
  const video = useRef<HTMLInputElement>(null)
  return <Modal open title="首页背景" description="换一张喜欢的照片，让首页更像你。" className="xm-background-modal" onOpenChange={open => { if (!open && !value.busy) close() }}>
    <p className="xm-background-hint">图片只保存在手机，离线也会保留。支持照片和较短的 GIF 动图；在首页可见时循环播放，切换页面或后台时自动显示静帧。GIF 限 8 MB、1600×1600、180 帧。</p>
    {value.src && <img className="xm-background-preview" src={value.poster ?? value.src} alt="当前首页背景预览" />}
    <input ref={input} type="file" accept="image/*" aria-label="选择首页背景图片" className="sr-only" disabled={value.busy} onChange={e => { const file = e.target.files?.[0]; e.target.value = ""; if (file) void value.save(file) }} />
    <button className="xm-primary" disabled={value.busy} onClick={() => input.current?.click()}><ImagePlus size={17} />{value.busy ? "正在保存…" : "从手机选择图片"}</button>
    <input ref={video} type="file" accept="video/mp4,video/webm" aria-label="选择首页背景视频" className="sr-only" disabled={value.busy} onChange={e => { const file = e.target.files?.[0]; e.target.value = ""; if (file) void value.save(file) }} />
    <button className="xq-button xm-background-reset" disabled={value.busy} onClick={() => video.current?.click()}>从手机选择短视频</button>
    <p className="xm-background-hint">短视频限 20 MB、20 秒、1080p，推荐 H.264 MP4。首页可见时静音循环播放；后台、切页或减少动态时停止播放。视频只存本机，不上传；视频原文件的元数据也会保留在本机。</p>
    <button className="xq-button xm-background-reset" disabled={value.busy || !value.src} onClick={() => void value.save(null)}><RotateCcw size={16} />恢复默认背景</button>
    {value.error && <p role="alert" className="xq-form-error">{value.error}</p>}
    <button className="xq-button xm-background-done" disabled={value.busy} onClick={close}>完成</button>
  </Modal>
}
