/** Bound encoded size, frame count and decoded work before handing a GIF to WebView. */
export function inspectGif(bytes: Uint8Array) {
  const bad = () => { throw new Error("IMAGE_FORMAT") }
  if (bytes.length < 14 || !["GIF87a", "GIF89a"].includes(String.fromCharCode(...bytes.slice(0, 6)))) bad()
  const word = (at: number) => bytes[at] | bytes[at + 1] << 8
  const width = word(6), height = word(8)
  if (!width || !height || width > 1600 || height > 1600) throw new Error("GIF_LIMIT")
  let at = 13 + (bytes[10] & 128 ? 3 * 2 ** ((bytes[10] & 7) + 1) : 0), frames = 0, decoded = 0
  const blocks = () => { while (at < bytes.length) { const count = bytes[at++]; if (!count) return; at += count; if (at > bytes.length) bad() } bad() }
  while (at < bytes.length) {
    const type = bytes[at++]
    if (type === 0x3b) { if (!frames) bad(); return { width, height, frames } }
    if (type === 0x21) { if (at >= bytes.length) bad(); at++; blocks(); continue }
    if (type !== 0x2c || at + 9 >= bytes.length) bad()
    const w = word(at + 4), h = word(at + 6), flags = bytes[at + 8]
    if (!w || !h || word(at) + w > width || word(at + 2) + h > height) bad()
    at += 9 + (flags & 128 ? 3 * 2 ** ((flags & 7) + 1) : 0)
    if (++frames > 180 || (decoded += w * h) > 40000000) throw new Error("GIF_LIMIT")
    at++; blocks()
  }
  return bad()
}

/** Normalize the loop extension once; WebView handles frames without a JS timer. */
export function loopingGif(bytes: Uint8Array): Blob {
  inspectGif(bytes)
  let at = 13 + (bytes[10] & 128 ? 3 * 2 ** ((bytes[10] & 7) + 1) : 0)
  const header = bytes.slice(0, at)
  header.set([0x38, 0x39, 0x61], 3)
  const chunks: BlobPart[] = [header, new Uint8Array([0x21, 0xff, 11, ...Array.from("NETSCAPE2.0", c => c.charCodeAt(0)), 3, 1, 0, 0, 0])]
  const skip = () => { while (bytes[at]) at += bytes[at] + 1; at++ }
  while (at < bytes.length) {
    const start = at, type = bytes[at++]
    if (type === 0x3b) { chunks.push(bytes.slice(start, at)); break }
    if (type === 0x21) {
      const label = bytes[at++], name = String.fromCharCode(...bytes.slice(at + 1, at + 12))
      const isLoop = label === 0xff && bytes[at] === 11 && ["NETSCAPE2.0", "ANIMEXTS1.0"].includes(name)
      skip(); if (!isLoop) chunks.push(bytes.slice(start, at))
    } else {
      const flags = bytes[at + 8]
      at += 9 + (flags & 128 ? 3 * 2 ** ((flags & 7) + 1) : 0)
      at++; skip(); chunks.push(bytes.slice(start, at))
    }
  }
  return new Blob(chunks, { type: "image/gif" })
}
