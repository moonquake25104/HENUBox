// Interactive HENU login helper (phase C, LOCAL_EDGE_INTERACTIVE).
//
// Contract with the Go parent (schoolconn/login.go):
//   - stdout carries JSON protocol lines only: {"event":"started"},
//     {"event":"cookies","url":...,"cookies":[...]}, {"event":"failed","message":...}.
//     stdin receives "ack" (cookies consumed) or "cancel".
//   - Cookie values exist only on this pipe. No traffic is intercepted, no
//     keystrokes recorded, no screenshots taken, no HAR/trace written.
//   - The Edge profile, disk cache and crash dumps live exclusively in the
//     task's dedicated directory, never in the user's daily profile.
import { createRequire } from 'node:module'
import { mkdirSync } from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import process from 'node:process'

function fail(message) {
  process.stdout.write(JSON.stringify({ event: 'failed', message }) + '\n')
  process.exit(1)
}

function arg(name) {
  const i = process.argv.indexOf('--' + name)
  return i >= 0 ? process.argv[i + 1] : undefined
}

const profile = arg('profile')
const loginURL = arg('url')
const origin = arg('origin')
const deadlineISO = arg('deadline')
const edgePath = arg('edge')
if (!profile || !loginURL || !origin || !edgePath) fail('缺少启动参数')
const deadline = deadlineISO ? Date.parse(deadlineISO) : Date.now() + 10 * 60 * 1000

const require = createRequire(path.join(path.dirname(fileURLToPath(import.meta.url)), 'package.json'))

let chromium
try {
  chromium = require('playwright-core').chromium
} catch {
  fail('登录助手缺少 playwright 依赖')
}

let originHost
try {
  originHost = new URL(origin).hostname
} catch {
  fail('origin 参数无效')
}

let context
try {
  mkdirSync(profile, { recursive: true })
  context = await chromium.launchPersistentContext(profile, {
    executablePath: edgePath,
    headless: false,
    viewport: { width: 1280, height: 860 },
    // The user types school credentials into this window; the helper must
    // never persist them anywhere.
    args: [
      '--disable-breakpad', '--disable-crash-reporter', '--no-first-run',
      '--no-default-browser-check', '--disable-features=PasswordLeakDetection',
      '--disable-save-password-bubble',
      `--disk-cache-dir=${path.join(profile, 'disk-cache')}`,
      `--crash-dumps-dir=${path.join(profile, 'crashes')}`,
    ],
  })
} catch (err) {
  fail('无法启动 Edge：' + (err?.code ?? 'launch error'))
}

process.stdout.write(JSON.stringify({ event: 'started' }) + '\n')

// Make this one authorized window distinguishable from other school tabs.
if (typeof context.addInitScript === 'function') {
  await context.addInitScript(({ tag }) => {
    document.addEventListener('DOMContentLoaded', () => {
      document.title = `${tag} · ${document.title}`
    }, { once: true })
  }, { tag: `HENUBox 授权 ${path.basename(profile).slice(0, 6)}` })
}

let finished = false
function finish(event) {
  if (finished) return
  finished = true
  process.stdout.write(JSON.stringify(event) + '\n')
}

function looksLoggedIn(url) {
  try {
    const u = new URL(url)
    if (u.protocol !== 'https:') return false
    if (u.hostname !== originHost) return false
    // Only the verified post-login landing page counts as a success
    // candidate; "same host and not the login page" would also match the CAS
    // logon endpoint or school error pages. The session's real validity is
    // still decided by the Go-side FetchTerm check.
    return u.pathname === '/frame/homes.action'
  } catch {
    return false
  }
}

async function schoolCookies(source = context) {
  // context.cookies returns a Promise: awaiting before filtering is the
  // contract with Playwright. Only same-site cookies are collected; a helper
  // bug cannot leak foreign domains into the parent.
  const cookies = await source.cookies(origin)
  return cookies.filter((c) => {
    const d = String(c.domain || '').replace(/^\./, '').toLowerCase()
    return d === originHost || d.endsWith('.' + originHost)
  }).map(c => ({
    name: c.name, value: c.value, domain: c.domain, path: c.path,
    secure: c.secure, httpOnly: c.httpOnly,
    // Playwright uses Unix seconds (-1 for session cookies). The Go wire
    // contract uses *time.Time, therefore RFC3339 or null, never a number.
    expires: c.expires === undefined || c.expires === -1
      ? null : new Date(c.expires * 1000).toISOString(),
  }))
}

// Register every event handler BEFORE the navigation await: a top-level
// await would otherwise delay these registrations until goto finishes, so a
// browser close during startup would turn into an unhandled rejection.
process.on('unhandledRejection', () => {
  // The close handler below reports the failure; nothing else to do here.
})

// Testability knob only: production keeps the 1500 ms default.
const POLL_MS = Number(process.env.HELPER_POLL_MS || 1500)
// Closing the browser must never block the helper's own exit: a wedged
// browser leaves the Go parent waiting on this process. The Go-side tree
// kill remains the backstop for the browser itself.
const CLOSE_TIMEOUT_MS = Number(process.env.HELPER_CLOSE_TIMEOUT_MS || 5000)
async function shutdown(code) {
  await Promise.race([
    context.close().catch(() => {}),
    new Promise((resolve) => setTimeout(resolve, CLOSE_TIMEOUT_MS)),
  ])
  process.exit(code)
}
// Consecutive transient-failure budget: browser startup and navigation can
// legitimately reject reads for a while, but an endless silent retry loop is
// how the sync/async bug hid for two full login windows.
const MAX_TRANSIENT_FAILURES = 20
let transientFailures = 0
// Safe heartbeat diagnostics on stderr (per-task log file, never stdout):
// only a coarse page-phase bucket and a cookie count. No URLs, no query
// strings, no cookie names or values. It turns a silent no-match window into
// a decisive one without widening the protocol.
let heartbeatTicks = 0
function pagePhase(url) {
  try {
    const u = new URL(url)
    if (u.hostname !== originHost) return 'other-host'
    if (u.pathname === '/frame/homes.action') return 'homes'
    if (u.pathname.includes('/cas/login') || u.pathname.includes('login.action')) return 'login'
    return 'other-same-host:' + u.pathname
  } catch {
    return 'invalid'
  }
}
function heartbeat(pagesSnapshot, cookieCount) {
  const phases = pagesSnapshot.map((p) => p.phase).join(',')
  console.error(JSON.stringify({ diag: 'poll', phases, cookieCount }))
}

const poll = setInterval(async () => {
  if (finished) return
  try {
    heartbeatTicks += 1
    if (Date.now() > deadline) {
      finish({ event: 'failed', message: '登录超时' })
      clearInterval(poll)
      await shutdown(2)
      return
    }
    // Read the live document, not only Playwright's cached navigation URL.
    // School CAS may replace its top-level document or open another page.
    // All contexts below belong to THIS dedicated browser, never daily Edge.
    const contexts = context.browser?.()?.contexts?.() ?? [context]
    const pages = contexts.flatMap(owner => owner.pages().map(page => ({ owner, page })))
    const snapshot = []
    let candidate
    for (const { owner, page } of pages) {
      const cached = page.url()
      let live = cached
      if (typeof page.evaluate === 'function') {
        try {
          const location = await page.evaluate(() => ({ origin: window.location.origin, pathname: window.location.pathname }))
          live = location.origin + location.pathname
        } catch {
          // A navigation can destroy the execution context. Do not accept a
          // stale cached success URL; retry the next tick instead.
          snapshot.push({ phase: 'navigating' })
          continue
        }
      }
      snapshot.push({ phase: pagePhase(live), cachedPhase: pagePhase(cached) })
      if (looksLoggedIn(live)) candidate = { owner, url: live }
    }
    if (heartbeatTicks % 5 === 0) {
      let n = -1
      try { n = (await schoolCookies()).length } catch { n = -1 }
      heartbeat(snapshot, n)
    }
    if (candidate && !finished) {
      const cookies = await schoolCookies(candidate.owner)
      if (cookies.length === 0) {
        // The one silent stall worth diagnosing immediately: the post-login
        // page is visible but the session cookies are not. Count only.
        console.error(JSON.stringify({ diag: 'homes-without-cookies', cookieCount: 0 }))
        return
      }
      if (!finished) {
        console.error(JSON.stringify({ diag: 'session-handoff', cookieCount: cookies.length }))
        finish({ event: 'cookies', url: candidate.url, cookies })
      }
      return
    }
    transientFailures = 0
  } catch (error) {
    // Programming errors must surface immediately as a safe failed event;
    // swallowing them until the deadline masks real helper bugs. Transient
    // browser state is retried, but only up to a bounded budget. The message
    // stays generic: no URLs, cookie data or personal values.
    if (error instanceof TypeError || error instanceof ReferenceError) {
      finish({ event: 'failed', message: '登录助手内部错误' })
      clearInterval(poll)
      await shutdown(4)
      return
    }
    transientFailures += 1
    if (transientFailures >= MAX_TRANSIENT_FAILURES) {
      finish({ event: 'failed', message: '登录助手无法读取会话状态' })
      clearInterval(poll)
      await shutdown(5)
      return
    }
  }
}, POLL_MS)

context.on('close', () => {
  clearInterval(poll)
  finish({ event: 'failed', message: '登录窗口已关闭' })
})

process.stdin.setEncoding('utf8')
process.stdin.on('data', async (chunk) => {
  const text = String(chunk).trim()
  if (text === 'cancel') {
    clearInterval(poll)
    finished = true
    await shutdown(0)
  }
  if (finished && text === 'ack') {
    clearInterval(poll)
    // The parent has the cookies; this process must not keep them anywhere.
    await shutdown(0)
  }
})
process.stdin.on('end', async () => {
  // Parent died or closed the pipe: shut the window down, leave nothing.
  clearInterval(poll)
  if (!finished) {
    finish({ event: 'failed', message: '登录助手与服务的连接中断' })
  }
  await shutdown(0)
})

// The browser starts on a blank tab; navigate it to the school login URL
// after every handler is live. The user does everything from there.
try {
  const page = context.pages()[0] ?? (await context.newPage())
  await page.goto(loginURL, { waitUntil: 'domcontentloaded', timeout: 30_000 })
} catch {
  if (!finished) {
    finish({ event: 'failed', message: '无法打开学校登录页' })
  }
  await shutdown(3)
}
