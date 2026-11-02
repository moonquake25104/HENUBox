import { BASE, getToken, readErrorMessage } from "./client"

export interface GenerateMorningBriefStreamOptions {
  force?: boolean
  onDelta: (delta: string) => void
  signal?: AbortSignal
}

function handleEvent(rawEvent: string, onDelta: (delta: string) => void): boolean {
  for (const line of rawEvent.split("\n")) {
    if (!line.startsWith("data:")) continue
    const payload = line.slice("data:".length).trim()
    if (payload === "") continue
    if (payload === "[DONE]") return true
    let parsed: unknown
    try {
      parsed = JSON.parse(payload)
    } catch {
      continue
    }
    if (parsed !== null && typeof parsed === "object") {
      const chunk = parsed as { delta?: unknown; error?: unknown }
      if (typeof chunk.error === "string") {
        throw new Error(chunk.error)
      }
      if (typeof chunk.delta === "string") {
        onDelta(chunk.delta)
      }
    }
  }
  return false
}

export async function generateMorningBriefStream(
  opts: GenerateMorningBriefStreamOptions,
): Promise<void> {
  const query = opts.force ? "?force=true" : ""
  const headers = new Headers({ Accept: "text/event-stream" })
  const token = getToken()
  if (token) {
    headers.set("Authorization", `Bearer ${token}`)
  }

  const res = await fetch(`${BASE}/api/ai/morning-brief/generate${query}`, {
    method: "POST",
    headers,
    signal: opts.signal,
  })

  if (!res.ok) {
    throw new Error(await readErrorMessage(res))
  }
  if (!res.body) {
    throw new Error("AI 晨报流不可用")
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ""

  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      let separator = buffer.indexOf("\n\n")
      while (separator !== -1) {
        const rawEvent = buffer.slice(0, separator)
        buffer = buffer.slice(separator + 2)
        if (handleEvent(rawEvent, opts.onDelta)) {
          await reader.cancel()
          return
        }
        separator = buffer.indexOf("\n\n")
      }
    }
  } finally {
    reader.releaseLock()
  }
}
