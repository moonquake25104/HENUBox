/** Validate before replacing the existing background; decoder failure leaves it intact. */
export async function prepareHomeVideo(file: File): Promise<{ media: Blob; poster: Blob }> {
  if (!/^video\/(mp4|webm)$/i.test(file.type)) throw new Error("VIDEO_FORMAT")
  if (!file.size || file.size > 20 * 1024 * 1024) throw new Error("VIDEO_LIMIT")
  const url = URL.createObjectURL(file), video = document.createElement("video")
  video.muted = true; video.playsInline = true; video.preload = "auto"
  try {
    await new Promise<void>((resolve, reject) => {
      const timeout = window.setTimeout(() => { clear(); reject(new Error("VIDEO_FORMAT")) }, 15000)
      const clear = () => { window.clearTimeout(timeout); video.onloadeddata = video.onerror = null }
      video.onloadeddata = () => { clear(); resolve() }
      video.onerror = () => { clear(); reject(new Error("VIDEO_FORMAT")) }
      video.src = url; video.load()
    })
    if (!Number.isFinite(video.duration) || video.duration < 0.1 || video.duration > 20 || !video.videoWidth || !video.videoHeight || Math.max(video.videoWidth, video.videoHeight) > 1920 || Math.min(video.videoWidth, video.videoHeight) > 1080) throw new Error("VIDEO_LIMIT")
    const ratio = Math.min(1, 1280 / Math.max(video.videoWidth, video.videoHeight)), canvas = document.createElement("canvas")
    canvas.width = Math.max(1, Math.round(video.videoWidth * ratio)); canvas.height = Math.max(1, Math.round(video.videoHeight * ratio))
    const ctx = canvas.getContext("2d"); if (!ctx) throw new Error("VIDEO_FORMAT")
    ctx.drawImage(video, 0, 0, canvas.width, canvas.height)
    const poster = await new Promise<Blob>((resolve, reject) => canvas.toBlob(b => b ? resolve(b) : reject(new Error("VIDEO_FORMAT")), "image/jpeg", 0.85))
    return { media: file.slice(0, file.size, file.type.toLowerCase()), poster }
  } finally { video.pause(); video.removeAttribute("src"); video.load(); URL.revokeObjectURL(url) }
}
