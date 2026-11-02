import { useEffect, useRef, useState } from "react"

export function HomeVideo({ src, poster, playing }: { src: string; poster: string | null; playing: boolean }) {
  const ref = useRef<HTMLVideoElement>(null), [failed, setFailed] = useState(false)
  useEffect(() => { setFailed(false) }, [src])
  useEffect(() => {
    const video = ref.current
    if (!video || !playing || failed) return
    let active = true
    video.muted = true
    void video.play().catch(() => { if (active) setFailed(true) })
    return () => { active = false; video.pause() }
  }, [src, playing, failed])
  if (!playing || failed) return poster ? <img className="xm-hero-photo" src={poster} alt="" /> : null
  return <video ref={ref} className="xm-hero-photo" src={src} poster={poster ?? undefined} muted playsInline loop autoPlay preload="metadata" aria-hidden="true" tabIndex={-1} onError={() => setFailed(true)} />
}
