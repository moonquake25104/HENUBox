import { useEffect, useRef, useState } from "react"
import { prepareHomeMedia, readHomeMedia, writeHomeBackground } from "@/lib/mobile/home-background"

export function useHomeBackground() {
  const [media, setMedia] = useState<{ src: string | null; poster: string | null; animated: boolean; video: boolean }>({ src: null, poster: null, animated: false, video: false })
  const [busy, setBusy] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState("")
  const alive = useRef(false)
  useEffect(() => {
    let active = true
    alive.current = true
    void readHomeMedia().then(({ media: blob, poster }) => {
      if (active && blob) setMedia({ src: URL.createObjectURL(blob), poster: poster ? URL.createObjectURL(poster) : null, animated: blob.type === "image/gif" || blob.type.startsWith("video/"), video: blob.type.startsWith("video/") })
    }).catch(() => { if (active) setError("暂时无法读取背景图片，请重试。") })
      .finally(() => { if (active) setLoading(false) })
    return () => { active = false; alive.current = false }
  }, [])
  useEffect(() => () => { if (media.src) URL.revokeObjectURL(media.src); if (media.poster) URL.revokeObjectURL(media.poster) }, [media])
  async function save(file: File | null) {
    if (busy || loading) return
    setBusy(true); setError("")
    try {
      const prepared = file ? await prepareHomeMedia(file) : null
      await writeHomeBackground(prepared?.media ?? null, prepared?.poster ?? null)
      if (alive.current) setMedia(prepared ? { src: URL.createObjectURL(prepared.media), poster: prepared.poster ? URL.createObjectURL(prepared.poster) : null, animated: prepared.media.type === "image/gif" || prepared.media.type.startsWith("video/"), video: prepared.media.type.startsWith("video/") } : { src: null, poster: null, animated: false, video: false })
    } catch (e) {
      if (alive.current && e instanceof Error && e.message.startsWith("VIDEO_")) { setError(e.message === "VIDEO_LIMIT" ? "请选择 20 MB、20 秒、1080p 以内的短视频。" : "视频无法解码，请换成 H.264 MP4 或 VP8 WebM。原背景仍保留。"); return }
      if (alive.current) setError(e instanceof Error && e.message === "GIF_LIMIT" ? "GIF 请控制在 8 MB、1600×1600 以内，最多 180 帧；请换一张较短的小动图。" : e instanceof Error && e.message === "IMAGE_SIZE" ? "请选择 25 MB 以内、尺寸较小的图片。" : e instanceof Error && e.message === "IMAGE_FORMAT" ? "这张图片无法读取，请换一张 JPG、PNG、WebP 或 GIF。" : "图片未能保存，原背景仍保留，请重试。")
    } finally { if (alive.current) setBusy(false) }
  }
  return { ...media, busy: busy || loading, error, save }
}
