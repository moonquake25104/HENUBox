import { Capacitor, registerPlugin } from "@capacitor/core"
import { GUIDE_PAGES, parseGuidePage, type DocumentKind, type GuideDocument, type GuideKind, guideAssetUrl } from "./guide-parser"
export { GUIDE_PAGES, type GuideKind, type GuideDocument, type DocumentKind } from "./guide-parser"
type Asset = { uri: string; width: number; height: number; bytes: number; stale: boolean }
interface GuideBridge {
  getPage(p: { kind: DocumentKind }): Promise<{ html: string }>
  getAsset(p: { url: string; refresh: boolean }): Promise<Asset>
  viewImage(p: { url: string; title: string }): Promise<void>
  openPage(p: { kind: GuideKind }): Promise<void>
  dial(p: { number: string }): Promise<void>
  saveVisitGuide(p: { url: string }): Promise<void>
  cacheInfo(): Promise<{ bytes: number }>
  clearCache(): Promise<void>
}
const bridge = registerPlugin<GuideBridge>("XiqueGuide"), TTL = 86400000
export const guideAvailable = () => Capacitor.isPluginAvailable("XiqueGuide")
function db(): Promise<IDBDatabase> { return new Promise((resolve, reject) => { const r = indexedDB.open("henubox-campus-guide-v1", 1); r.onupgradeneeded = () => r.result.createObjectStore("documents", { keyPath: "kind" }); r.onsuccess = () => resolve(r.result); r.onerror = () => reject(r.error) }) }
async function store<T>(mode: IDBTransactionMode, action: (s: IDBObjectStore) => IDBRequest<T>): Promise<T> { const d = await db(); try { return await new Promise((resolve, reject) => { const tx = d.transaction("documents", mode), request = action(tx.objectStore("documents")); tx.oncomplete = () => resolve(request.result); tx.onerror = () => reject(tx.error); tx.onabort = () => reject(tx.error) }) } finally { d.close() } }
export async function readGuide(kind: DocumentKind): Promise<GuideDocument | null> { try { const d = await store<GuideDocument | undefined>("readonly", s => s.get(kind)); return d?.version === 1 && d.kind === kind && Number.isFinite(d.fetchedAt) && Array.isArray(d.images) && Array.isArray(d.sections) && Array.isArray(d.phones) ? d : null } catch { return null } }
const pending = new Map<string, Promise<GuideDocument>>()
let cacheEpoch = 0
export async function loadGuide(kind: DocumentKind, refresh = false): Promise<{ document: GuideDocument; stale: boolean; cache: boolean }> {
  const epoch = cacheEpoch
  const cached = await readGuide(kind)
  if (!refresh && cached && Date.now() - cached.fetchedAt < TTL) return { document: cached, stale: false, cache: true }
  const key = kind
  try {
    let job = pending.get(key)
    if (!job) { job = (async () => {
      let html: string
      if (guideAvailable()) html = (await bridge.getPage({ kind })).html
      else { const response = await fetch(GUIDE_PAGES[kind].url, { credentials: "omit", signal: AbortSignal.timeout(15000) }); if (!response.ok) throw new Error("RESOURCE_UNAVAILABLE"); html = await response.text() }
      const parsed = parseGuidePage(kind, html)
      if (epoch === cacheEpoch) await store("readwrite", s => s.put(parsed))
      return parsed
    })(); pending.set(key, job); void job.finally(() => { if (pending.get(key) === job) pending.delete(key) }).catch(() => {}) }
    return { document: await job, stale: false, cache: false }
  } catch (e) { if (cached) return { document: cached, stale: true, cache: true }; throw e }
}
const assets = new Map<string, Promise<Asset>>()
export async function guideAsset(url: string, refresh = false): Promise<Asset> {
  if (!guideAvailable() || !guideAssetUrl(url)) throw new Error("RESOURCE_UNAVAILABLE")
  const key = url + ":" + refresh
  if (!assets.has(key)) { const job = bridge.getAsset({ url, refresh }).then(a => { const uri = Capacitor.convertFileSrc(a.uri); return { ...a, uri: uri.startsWith("https://localhost/") ? `${uri}?guide=${Date.now()}` : uri } }); assets.set(key, job); void job.finally(() => { if (assets.get(key) === job) assets.delete(key) }).catch(() => {}) }
  return assets.get(key)!
}
export async function openGuideSource(kind: GuideKind) { if (guideAvailable()) await bridge.openPage({ kind }); else window.open(GUIDE_PAGES[kind].url, "_blank", "noopener,noreferrer") }
export const viewGuideImage = (url: string, title: string) => bridge.viewImage({ url, title })
export const dialGuidePhone = (number: string) => bridge.dial({ number })
export const saveVisitGuide = (url: string) => bridge.saveVisitGuide({ url })
export async function guideCacheInfo() { const docs = await store<number>("readonly", s => s.count()); return { documents: docs, bytes: guideAvailable() ? (await bridge.cacheInfo()).bytes : 0 } }
export async function clearGuideCache() { cacheEpoch++; pending.clear(); assets.clear(); if (guideAvailable()) await bridge.clearCache(); await store("readwrite", s => s.clear()) }
