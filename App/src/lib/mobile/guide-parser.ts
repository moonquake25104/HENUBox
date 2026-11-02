export const GUIDE_PAGES = {
  calendar: { title: "校历", url: "https://www.henu.edu.cn/xyfw/xl.htm", detail: "学期安排，随时查阅" },
  maps: { title: "校园地图", url: "https://www.henu.edu.cn/zjhd/xqsyt.htm", detail: "郑州 · 金明 · 明伦" },
  phones: { title: "校内电话", url: "https://www.henu.edu.cn/xyfw/xndh.htm", detail: "查部门，找联系方式" },
  shuttle: { title: "校车查询", url: "https://www.henu.edu.cn/xyfw/xccx.htm", detail: "线路与官方发车时刻" },
  visits: { title: "预约进校", url: "https://www.henu.edu.cn/xyfw/yyjx.htm", detail: "查看流程，在微信办理" },
  vr: { title: "校史馆 VR 全景", url: "https://www.720yun.com/vr/e4b2f9rfu4a", detail: "走进河大的百年故事" },
  emblem: { title: "校徽校训校风", url: "https://www.henu.edu.cn/zjhd/xhxxxf.htm", detail: "认识河大的精神与标识" },
  anthem: { title: "河南大学校歌", url: "https://www.henu.edu.cn/zjhd/hndxxg.htm", detail: "听见河大，读懂传承" },
} as const
export type GuideKind = keyof typeof GUIDE_PAGES
export type DocumentKind = Exclude<GuideKind, "vr">
export type GuideImage = { url: string; title: string }
export type GuideSection = { title: string; paragraphs: string[]; images: GuideImage[] }
export type GuidePhone = { department: string; internal: string; external: string; dial: string[] }
export type GuideDocument = { version: 1; kind: DocumentKind; fetchedAt: number; images: GuideImage[]; sections: GuideSection[]; phones: GuidePhone[]; audio?: string }
export const ANTHEM_AUDIO = "https://www.henu.edu.cn/mp3/xiaoge.mp3"
const text = (e: Element) => { const c = e.cloneNode(true) as Element; c.querySelectorAll("br").forEach(b => b.replaceWith(" ")); return (c.textContent || "").replace(/\s+/g, " ").trim() }
export function guideAssetUrl(value: string): boolean {
  try { const u = new URL(value); return u.protocol === "https:" && u.hostname === "www.henu.edu.cn" && !u.port && !u.username && !u.password && !u.search && !u.hash && !u.pathname.includes("%") && (value === ANTHEM_AUDIO || /^\/(?:images|__local)\/[A-Za-z0-9_./-]+\.(?:png|jpe?g|gif)$/.test(u.pathname)) } catch { return false }
}
function image(e: Element, base: string, title: string): GuideImage | null {
  try { const url = new URL(e.getAttribute("src") || "", base).href; return guideAssetUrl(url) && url !== ANTHEM_AUDIO ? { url, title } : null } catch { return null }
}
const unique = (items: GuideImage[]) => items.filter((im, i) => items.findIndex(v => v.url === im.url) === i)
const labels = new Map([
  ["DF3ACBBAB2E0E9A62FE786F8EFA_9371D518_4E937.jpg", "2025—2026 学年 · 春季学期"],
  ["673FD79A2B926AF48611CAF1675_5DC70943_54A46.jpg", "2026—2027 学年 · 秋季学期"],
])
export function parseGuidePage(kind: DocumentKind, html: string, now = Date.now()): GuideDocument {
  if (html.length > 1024 * 1024) throw new Error("GUIDE_FORMAT_CHANGED")
  // Template contents stay inert: even image/iframe subresources are not loaded.
  const template = document.createElement("template"); template.innerHTML = html
  const doc = template.content, body = doc.querySelector(".nyrcon"), base = GUIDE_PAGES[kind].url
  if (!body || !doc.querySelector("title")?.textContent?.includes("河南大学")) throw new Error("GUIDE_FORMAT_CHANGED")
  // HTML is only inspected, never mounted. Remove executable and irrelevant nodes.
  body.querySelectorAll("script:not([type]),script[type],style,iframe,object,embed").forEach(e => { if (kind === "shuttle" && e.tagName === "SCRIPT") return; e.remove() })
  const result: GuideDocument = { version: 1, kind, fetchedAt: now, images: [], sections: [], phones: [] }
  if (kind === "maps") {
    const names = [...body.querySelectorAll(".hd li span")].map(text)
    result.images = [...body.querySelectorAll(".tabCon img")].flatMap((e, i) => { const v = image(e, base, names[i] || `校区示意图 ${i + 1}`); return v ? [v] : [] })
    if (result.images.length !== 3 || !["郑州", "金明", "明伦"].every(n => result.images.some(im => im.title.includes(n)))) throw new Error("GUIDE_FORMAT_CHANGED")
  } else if (kind === "phones") {
    const table = body.querySelector("table"), header = table?.querySelector("tr"), area = header && /外线[（(](0\d{2,3})[）)]/.exec(text(header))?.[1]
    if (!header || !text(header).includes("部门") && !text(header).includes("部 门")) throw new Error("GUIDE_FORMAT_CHANGED")
    result.phones = [...table!.querySelectorAll("tr")].slice(1).flatMap(row => {
      const cells = [...row.querySelectorAll("td")].map(text); if (cells.length !== 3 || !cells[0] || cells[0].length > 160) return []
      const external = cells[2], numbers = external.match(/(?:0\d{2,3}[-—－ ]?)?\d{7,8}|1[3-9]\d{9}|400\d{7}/g) || []
      const dial = numbers.map(n => n.replace(/[-—－ ]/g, "")).map(n => /^\d{7,8}$/.test(n) && area ? area + n : n).filter(n => /^(?:0\d{9,11}|1\d{10}|400\d{7})$/.test(n))
      return [{ department: cells[0], internal: cells[1], external, dial: [...new Set(dial)] }]
    })
    if (result.phones.length < 5) throw new Error("GUIDE_FORMAT_CHANGED")
  } else if (kind === "shuttle") {
    const script = [...body.querySelectorAll("script")].map(e => e.textContent || "").find(s => s.includes("vsb_pdf_image_data")), match = script && /vsb_pdf_image_data\s*=\s*(\[[^;]+\])\s*;/.exec(script)
    if (!match) throw new Error("GUIDE_FORMAT_CHANGED")
    const paths: unknown = JSON.parse(match[1]); if (!Array.isArray(paths) || paths.length > 20) throw new Error("GUIDE_FORMAT_CHANGED")
    const titles = ["郑开三校区 · 工作日", "郑开三校区 · 周末与假期", "开封校区 · 工作日", "开封校区 · 周末", "开封校区 · 假期", "郑汴高校专线"]
    const known = ["5C47F1FB3697F37BEB66F9026B3", "AFA055EB3907E3CC2470D7DE742", "CD75A2B378871418C02BAD4E2C1", "31E5C2A29FF4651FEBD45F5951C", "83BD0A8E83F7A829B7F07CAF725", "B328EAB04AD1A595DF7BE71FE65"]
    result.images = paths.flatMap((p, i) => { if (typeof p !== "string") return []; const url = new URL(p, base).href; return guideAssetUrl(url) ? [{ url, title: known[i] && url.includes(known[i]) ? titles[i] : `官方时刻表 · 第 ${i + 1} 页` }] : [] })
    if (!result.images.length) throw new Error("GUIDE_FORMAT_CHANGED")
  } else if (kind === "emblem") {
    result.sections = [...body.children].filter(e => e.querySelector(".xtit")).map(e => ({ title: text(e.querySelector(".xtit")!), paragraphs: [...e.querySelectorAll(".desc p")].map(text).filter(Boolean), images: [...e.querySelectorAll("img")].flatMap(im => { const v = image(im, base, text(e.querySelector(".xtit")!)); return v ? [v] : [] }) }))
    if (!result.sections.some(s => s.title === "校徽") || !result.sections.some(s => s.title === "校训") || !result.sections.some(s => s.title === "校风")) throw new Error("GUIDE_FORMAT_CHANGED")
  } else {
    const content = body.querySelector('[id="vsb_content_2"],[id="vsb_content"],[id="vsb_content_4"]') || body
    result.images = unique([...content.querySelectorAll("img")].flatMap((e, i) => { const v = image(e, base, kind === "visits" ? "校园访客预约流程" : kind === "calendar" ? "" : `校歌官方图文 ${i + 1}`); if (!v) return []; if (kind === "calendar") v.title = labels.get(v.url.split("/").at(-1)!) || `官方校历 · 图 ${i + 1}`; return [v] }))
    if (kind === "anthem") {
      const src = body.querySelector("[data-src]")?.getAttribute("data-src"); if (src && new URL(src, base).href === ANTHEM_AUDIO) result.audio = ANTHEM_AUDIO
      const paragraphs = [...body.querySelectorAll(".arc-con p")].map(text).filter(Boolean)
      result.sections = [{ title: "校歌的故事", paragraphs, images: [] }]
      if (!result.audio) throw new Error("GUIDE_FORMAT_CHANGED")
    }
    if (!result.images.length) throw new Error("GUIDE_FORMAT_CHANGED")
  }
  return result
}
