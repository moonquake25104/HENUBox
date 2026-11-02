// The selected image is stored in this app WebView's IndexedDB. No upload,
// external path, library permission or school account is involved.
import { inspectGif, loopingGif } from "./gif"
import { prepareHomeVideo } from "./home-video"
const DATABASE = "xique-appearance-v1"
async function database(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const request = indexedDB.open(DATABASE, 1)
    request.onupgradeneeded = () => request.result.createObjectStore("images")
    request.onsuccess = () => { request.result.onversionchange = () => request.result.close(); resolve(request.result) }
    request.onerror = () => reject(new Error("IMAGE_STORAGE"))
    request.onblocked = () => reject(new Error("IMAGE_STORAGE"))
  })
}
export async function readHomeMedia(): Promise<{ media: Blob | null; poster: Blob | null }> {
  const db = await database()
  try {
    return await new Promise((resolve, reject) => {
      const transaction = db.transaction("images", "readonly")
      const request = transaction.objectStore("images").get("home-hero")
      const poster = transaction.objectStore("images").get("home-hero-poster")
      transaction.oncomplete = () => { const media = request.result instanceof Blob ? request.result : null, still = poster.result instanceof Blob ? poster.result : null; if (media?.type === "image/gif") void media.arrayBuffer().then(data => resolve({ media: loopingGif(new Uint8Array(data)), poster: still })).catch(reject); else resolve({ media, poster: still }) }
      transaction.onerror = transaction.onabort = () => reject(new Error("IMAGE_STORAGE"))
    })
  } finally { db.close() }
}
export async function readHomeBackground(): Promise<Blob | null> { return (await readHomeMedia()).media }
export async function writeHomeBackground(image: Blob | null, poster: Blob | null = null): Promise<void> {
  const db = await database()
  try {
    await new Promise<void>((resolve, reject) => {
      const transaction = db.transaction("images", "readwrite")
      const store = transaction.objectStore("images")
      if (image) store.put(image, "home-hero")
      else store.delete("home-hero")
      if (poster) store.put(poster, "home-hero-poster")
      else store.delete("home-hero-poster")
      transaction.oncomplete = () => resolve()
      transaction.onerror = transaction.onabort = () => reject(new Error("IMAGE_STORAGE"))
    })
  } finally { db.close() }
}

export async function prepareHomeMedia(file: File): Promise<{ media: Blob; poster: Blob | null }> {
  if (file.type.startsWith("video/")) return prepareHomeVideo(file)
  if (!/^image\/(jpeg|png|webp|gif|avif|heic|heif)$/i.test(file.type)) throw new Error("IMAGE_FORMAT")
  if (!file.size || file.size > 25 * 1024 * 1024) throw new Error("IMAGE_SIZE")
  const gif = file.type.toLowerCase() === "image/gif"
  if (gif) { if (file.size > 8 * 1024 * 1024) throw new Error("GIF_LIMIT"); inspectGif(new Uint8Array(await file.arrayBuffer())) }
  const url = URL.createObjectURL(file)
  try {
    const image = new Image()
    image.src = url
    try { await image.decode() } catch { throw new Error("IMAGE_FORMAT") }
    if (!image.naturalWidth || !image.naturalHeight || image.naturalWidth * image.naturalHeight > 80000000) throw new Error("IMAGE_SIZE")
    const ratio = Math.min(1, 1600 / Math.max(image.naturalWidth, image.naturalHeight))
    const canvas = document.createElement("canvas")
    canvas.width = Math.max(1, Math.round(image.naturalWidth * ratio))
    canvas.height = Math.max(1, Math.round(image.naturalHeight * ratio))
    const context = canvas.getContext("2d")
    if (!context) throw new Error("IMAGE_FORMAT")
    context.fillStyle = "#ffffff"
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(image, 0, 0, canvas.width, canvas.height)
    // Re-encode as a bounded static JPEG; original EXIF/location is not copied.
    const still = await new Promise<Blob>((resolve, reject) => canvas.toBlob(blob => blob ? resolve(blob) : reject(new Error("IMAGE_FORMAT")), "image/jpeg", 0.86))
    return { media: gif ? loopingGif(new Uint8Array(await file.arrayBuffer())) : still, poster: gif ? still : null }
  } finally { URL.revokeObjectURL(url) }
}
export async function prepareHomeBackground(file: File): Promise<Blob> { return (await prepareHomeMedia(file)).media }
