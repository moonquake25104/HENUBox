// Module interception hook: loaded via `node --require` so helper.mjs resolves
// our fake playwright-core instead of the real one. Test-only; never shipped
// in a production launch path.
const Module = require('node:module')
const path = require('node:path')
const FAKE = path.join(__dirname, 'fake-playwright.cjs')
const origResolve = Module._resolveFilename
Module._resolveFilename = function (request, ...args) {
  if (request === 'playwright-core') return FAKE
  return origResolve.call(this, request, ...args)
}
