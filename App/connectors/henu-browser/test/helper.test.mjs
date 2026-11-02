// Helper-layer tests: run helper.mjs as a real child process with an injected
// fake playwright whose browser APIs have the real async shapes. No real
// browser, no school request, no database; every cookie value here is
// synthetic. Run: node --test test/helper.test.mjs
import { test } from 'node:test'
import assert from 'node:assert/strict'
import { spawn } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import path from 'node:path'
import { fileURLToPath } from 'node:url'

const here = path.dirname(fileURLToPath(import.meta.url))
const projectRoot = path.join(here, '..')
const helper = path.join(projectRoot, 'helper.mjs')
const hook = path.join(here, 'fake-playwright-hook.cjs')
const LOGIN = 'https://xk.henu.edu.cn/cas/login.action'
const HOME = 'https://xk.henu.edu.cn/frame/homes.action?v=synthetic'
const POLL_MS = 40

// Spawns the helper with the fake playwright and returns a controller that
// collects stdout JSON events, allows stdin writes and waits for exit.
function runHelper({ env, stdinAfter } = {}) {
  const dir = mkdtempSync(path.join(tmpdir(), 'henu-helper-test-'))
  const marker = path.join(dir, 'marker.log')
  const child = spawn(process.execPath, [
    '--require', hook, helper,
    '--profile', path.join(dir, 'profile'),
    '--url', env?.url ?? LOGIN,
    '--origin', 'https://xk.henu.edu.cn',
    '--deadline', env?.deadline ?? new Date(Date.now() + 60_000).toISOString(),
    '--edge', path.join(dir, 'msedge.exe'),
  ], {
    cwd: projectRoot,
    env: {
      ...process.env,
      HELPER_POLL_MS: String(POLL_MS),
      HELPER_CLOSE_TIMEOUT_MS: '300',
      FAKE_MARKER: marker,
      ...env?.fake,
    },
    stdio: ['pipe', 'pipe', 'pipe'],
  })
  const events = []
  let stderr = ''
  child.stdout.setEncoding('utf8')
  child.stdout.on('data', (chunk) => {
    for (const line of chunk.split('\n')) {
      const t = line.trim()
      if (!t) continue
      try { events.push(JSON.parse(t)) } catch { /* non-protocol output */ }
    }
  })
  child.stderr.setEncoding('utf8')
  child.stderr.on('data', (c) => { stderr += c })
  if (stdinAfter) for (const [delay, text] of stdinAfter) {
    setTimeout(() => child.stdin.write(text + '\n'), delay)
  }
  const done = new Promise((resolve) => {
    child.on('exit', (code, signal) => {
      let markerText = ''
      try { markerText = readFileSync(marker, 'utf8') } catch { /* none */ }
      resolve({ code, signal, events, stderr, markerText })
      rmSync(dir, { recursive: true, force: true })
    })
  })
  return { child, events, done }
}

function waitFor(predicate, timeoutMs = 8000, stepMs = 20) {
  return new Promise((resolve, reject) => {
    const started = Date.now()
    const timer = setInterval(() => {
      if (predicate()) { clearInterval(timer); resolve() }
      if (Date.now() - started > timeoutMs) {
        clearInterval(timer)
        reject(new Error('waitFor timed out'))
      }
    }, stepMs)
  })
}

test('logged-in page yields exactly one cookies event, ack exits cleanly', async () => {
  const run = runHelper({
    env: { fake: { FAKE_URL_SEQUENCE: `${LOGIN};${HOME}`, FAKE_COOKIES: 'ok' } },
    stdinAfter: [[1200, 'ack']],
  })
  await waitFor(() => run.events.some((e) => e.event === 'cookies'))
  // Give a possible duplicate a chance to fire; the contract is exactly one.
  await new Promise((r) => setTimeout(r, 400))
  const cookieEvents = run.events.filter((e) => e.event === 'cookies')
  assert.equal(cookieEvents.length, 1, `expected exactly one cookies event, got ${cookieEvents.length}`)
  // Foreign-domain cookies must be filtered out by the collector.
  const names = cookieEvents[0].cookies.map((c) => c.name).sort()
  assert.deepEqual(names, ['JSESSIONID', 'route'])
  assert.equal(cookieEvents[0].cookies.find(c => c.name === 'JSESSIONID').expires, null)
  assert.equal(cookieEvents[0].cookies.find(c => c.name === 'route').expires, new Date(1793031296.25 * 1000).toISOString())
  assert.equal(run.events[0].event, 'started')
  const result = await run.done
  assert.equal(result.code, 0)
  assert.ok(result.markerText.includes('close-called'), 'browser close was requested')
  assert.ok(result.markerText.includes('goto:' + LOGIN), 'login page was navigated to')
})

test('empty cookies first: keeps polling, then succeeds once visible', async () => {
  const run = runHelper({
    env: { fake: { FAKE_URL_SEQUENCE: HOME, FAKE_COOKIES: 'empty-then-ok' } },
    stdinAfter: [[1200, 'ack']],
  })
  await waitFor(() => run.events.some((e) => e.event === 'cookies'))
  const result = await run.done
  assert.equal(result.code, 0)
  assert.equal(run.events.filter((e) => e.event === 'cookies').length, 1)
})

test('live document success is detected even when cached page URL stays on login', async () => {
  const run = runHelper({
    env: { fake: { FAKE_URL_SEQUENCE: LOGIN, FAKE_DOCUMENT_URL: HOME, FAKE_COOKIES: 'ok' } },
    stdinAfter: [[1200, 'ack']],
  })
  await waitFor(() => run.events.some(e => e.event === 'cookies'))
  const result = await run.done
  assert.equal(result.code, 0)
  assert.equal(result.events.filter(e => e.event === 'cookies').length, 1)
  assert.equal(new URL(result.events.find(e => e.event === 'cookies').url).pathname, '/frame/homes.action')
})

test('transient cookie rejection is retried, not reported as internal error', async () => {
  const run = runHelper({
    env: { fake: { FAKE_URL_SEQUENCE: HOME, FAKE_COOKIES: 'reject' } },
    stdinAfter: [[900, 'cancel']],
  })
  const result = await run.done
  // Cancellation wins over the retry budget; no internal-error event.
  assert.equal(result.code, 0)
  assert.ok(!run.events.some((e) => e.event === 'failed' && e.message === '登录助手内部错误'))
  assert.ok(result.markerText.split('\n').filter((l) => l.startsWith('cookies-read')).length > 1,
    'collector was retried across polls')
})

test('collector TypeError surfaces immediately as a safe failed event', async () => {
  const run = runHelper({
    env: { fake: { FAKE_URL_SEQUENCE: HOME, FAKE_COOKIES: 'typeerror' } },
  })
  const result = await run.done
  assert.equal(result.code, 4)
  const failed = run.events.find((e) => e.event === 'failed')
  assert.ok(failed, 'failed event emitted')
  assert.equal(failed.message, '登录助手内部错误')
  // Safety: the failure payload must not carry URL or cookie material.
  assert.equal(failed.url, undefined)
  assert.equal(failed.cookies, undefined)
})

test('login page alone never triggers a cookies event; cancel exits 0', async () => {
  const run = runHelper({
    env: { fake: { FAKE_URL_SEQUENCE: LOGIN, FAKE_COOKIES: 'ok' } },
    stdinAfter: [[800, 'cancel']],
  })
  const result = await run.done
  assert.equal(result.code, 0)
  assert.ok(!run.events.some((e) => e.event === 'cookies'),
    'CAS logon page must not be treated as logged in')
  assert.ok(!run.events.some((e) => e.event === 'failed'), 'no failure on clean cancel')
  assert.ok(result.markerText.includes('close-called'))
})

test('deadline expiry reports timeout and exits 2', async () => {
  const run = runHelper({
    env: {
      deadline: new Date(Date.now() - 1_000).toISOString(),
      fake: { FAKE_URL_SEQUENCE: LOGIN },
    },
  })
  const result = await run.done
  assert.equal(result.code, 2)
  const failed = run.events.find((e) => e.event === 'failed')
  assert.equal(failed?.message, '登录超时')
})

test('wedged browser close cannot block helper exit after ack', async () => {
  const run = runHelper({
    env: { fake: { FAKE_URL_SEQUENCE: HOME, FAKE_COOKIES: 'ok', FAKE_CLOSE_MODE: 'hang' } },
    stdinAfter: [[1200, 'ack']],
  })
  await waitFor(() => run.events.some((e) => e.event === 'cookies'))
  const result = await run.done
  assert.equal(result.code, 0, 'helper must exit despite close() never resolving')
})

test('navigation failure reports a safe failed event and exits 3', async () => {
  const run = runHelper({
    env: { fake: { FAKE_GOTO_MODE: 'fail' } },
  })
  const result = await run.done
  assert.equal(result.code, 3)
  assert.equal(run.events.find((e) => e.event === 'failed')?.message, '无法打开学校登录页')
})
