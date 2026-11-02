// Fake playwright-core for helper-layer tests. The browser APIs here are the
// REAL async shapes (Promises, callbacks registered via on()), never
// synchronous array stubs — that distinction is exactly what the original bug
// hid. Behaviour is driven by env vars set by the test:
//   FAKE_URL_SEQUENCE  semicolon-separated page URLs handed out per poll tick
//                      (the last one repeats); default stays on the login page
//   FAKE_COOKIES       ok | empty-then-ok | reject | typeerror
//   FAKE_CLOSE_MODE    ok | hang          (close() resolves / never resolves)
//   FAKE_GOTO_MODE     ok | fail
// A marker file (FAKE_MARKER path) records close() calls so the test can
// assert the browser was actually asked to shut down.
const fs = require('node:fs')

function env(name, fallback) {
  const v = process.env[name]
  return v === undefined || v === '' ? fallback : v
}

function marker(text) {
  const p = process.env.FAKE_MARKER
  if (p) fs.appendFileSync(p, text + '\n')
}

function urlSequence() {
  const raw = env('FAKE_URL_SEQUENCE', 'https://xk.henu.edu.cn/cas/login.action')
  const list = raw.split(';').filter(Boolean)
  return list.length ? list : ['https://xk.henu.edu.cn/cas/login.action']
}

function makeCookies() {
  const mode = env('FAKE_COOKIES', 'ok')
  const synthetic = [
    { domain: 'xk.henu.edu.cn', name: 'JSESSIONID', value: 'synthetic-session', path: '/', secure: false, httpOnly: true, expires: -1 },
    { domain: '.xk.henu.edu.cn', name: 'route', value: 'synthetic-route', path: '/', secure: false, httpOnly: true, expires: 1793031296.25 },
    { domain: 'other.example', name: 'foreign', value: 'must-not-leak', path: '/', secure: false, httpOnly: false },
  ]
  if (mode === 'typeerror') {
    return async () => {
      throw new TypeError('synthetic programming error')
    }
  }
  if (mode === 'reject') {
    return async () => {
      throw new Error('target closed while reading cookies (synthetic transient)')
    }
  }
  if (mode === 'empty-then-ok') {
    let calls = 0
    return async () => {
      calls += 1
      if (calls === 1) return []
      return synthetic
    }
  }
  return async () => synthetic
}

module.exports = {
  chromium: {
    async launchPersistentContext() {
      const urls = urlSequence()
      let tick = 0
      const cookiesFn = makeCookies()
      const page = {
        url: () => urls[Math.min(tick, urls.length - 1)],
        async evaluate() {
          const u = new URL(env('FAKE_DOCUMENT_URL', urls[Math.min(tick, urls.length - 1)]))
          return { origin: u.origin, pathname: u.pathname }
        },
        async goto(url) {
          marker('goto:' + url)
          if (env('FAKE_GOTO_MODE', 'ok') === 'fail') throw new Error('synthetic navigation failure')
        },
      }
      const context = {
        // Each poll tick calls pages() first; that advances the synthetic URL
        // timeline so the page can "move" from the login page to the
        // post-login landing page without a real browser.
        pages: () => {
          tick += 1
          return [page]
        },
        cookies: async (origin) => {
          marker('cookies-read:' + origin)
          return cookiesFn()
        },
        on: () => {},
        async close() {
          marker('close-called')
          if (env('FAKE_CLOSE_MODE', 'ok') === 'hang') {
            await new Promise(() => {}) // never resolves: exercises the exit timeout
          }
        },
      }
      marker('launched')
      return context
    },
  },
}
