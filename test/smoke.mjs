// End-to-end smoke test in a real browser.
//
// Drives every route as guest, owner and builder, and fails on console errors,
// page exceptions, failed requests or 4xx/5xx responses. Each route also
// asserts something only a correctly rendered page can show — "not blank" is
// too weak a check, because the layout map still renders its chrome when every
// plot polygon is missing.
//
//   node test/smoke.mjs https://plotting-society.aniketcharjan3.workers.dev
//   node test/smoke.mjs http://localhost:5173
//
// Needs Playwright and a Chrome install: npx playwright install chrome
//
// Exits non-zero on the first sign of trouble, so it works as a release gate.
import { chromium } from 'playwright'

const BASE = (process.argv[2] ?? 'http://localhost:5173').replace(/\/$/, '')
const PASSWORD = process.env.SMOKE_PASSWORD ?? 'sandesh-demo-2026'

const LOGIN = {
  owner: 'rohit.deshmukh@example.com',
  builder: 'office@shivrudragroup.in',
}

// Each route declares a `ready` predicate that runs IN THE BROWSER and is
// polled until true. It must assert something only a correct render produces.
//
// Predicates compare against lowercased innerText via txt(). innerText returns
// text as RENDERED, so a label styled `text-transform: uppercase` comes back as
// "CLOSING BALANCE" and a case-sensitive match on "Closing balance" fails on a
// page that is entirely correct.
//
// Two traps this avoids. Waiting for text that also appears in the nav — "Layout
// map" is a nav link — succeeds instantly and measures nothing. And asserting
// once after a fixed sleep races a cold free-tier API, which can take 30
// seconds to answer the first request.
const ROUTES = {
  guest: [
    ['/explore', () => {
      const t = txt()
      // 823 comes from the highlights array and club house from amenities.
      // Both are jsonb, and both render only if they arrived as JSON rather
      // than as the base64 string a []byte field would have produced.
      return t.includes('sandesh nagari 7') && t.includes('823') && t.includes('club house')
    }],
  ],
  owner: [
    // The map's whole job is drawing the plots. The chrome renders with or
    // without them, so counting them is the only check that means anything —
    // via data-plot rather than a tag name, which a restyle would break.
    ['/', () => document.querySelectorAll('svg [data-plot]').length > 100],
    ['/my-plot', () => txt().includes('one-time maintenance') && /sq ft ×/.test(txt())],
    ['/updates', () => txt().includes('site progress')],
    ['/fund', () => txt().includes('closing balance') && txt().includes('ledger')],
    ['/queries', () => txt().includes('my queries')],
    ['/queries/new', () => txt().includes('raise a query')],
  ],
  builder: [
    ['/admin', () => txt().includes('past sla')],
    ['/admin/inbox', () => txt().includes('query inbox')],
    ['/admin/leads', () => txt().includes('converted')],
    ['/admin/plots', () => document.querySelectorAll('table tbody tr').length > 10],
    ['/admin/updates', () => txt().includes('post a site update')],
    ['/admin/fund', () => txt().includes('add an entry')],
    // The reference quote proves the per-sector rates resolved.
    ['/admin/maintenance', () => txt().includes('1,540 sq ft')],
  ],
}

const problems = []
const record = (role, route, kind, text) =>
  problems.push({ role, route, kind, text: String(text).replace(/\s+/g, ' ').slice(0, 260) })

// Chrome reports ERR_NETWORK_CHANGED / ERR_NETWORK_IO_SUSPENDED when the
// machine's network flaps — a VPN reconnecting, a proxy cycling, a laptop
// waking. Those are not the app failing, and a gate that cries wolf about them
// gets ignored, so transport errors are retried and only a persistent one is
// reported.
const TRANSIENT = /ERR_NETWORK_CHANGED|ERR_NETWORK_IO_SUSPENDED|ERR_CONNECTION_RESET|ERR_NETWORK_ACCESS_DENIED/

async function goto(page, url, attempts = 3) {
  for (let i = 1; ; i++) {
    try {
      return await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 90000 })
    } catch (e) {
      if (i >= attempts || !TRANSIENT.test(String(e))) throw e
      await page.waitForTimeout(3000 * i)
    }
  }
}

function listen(page, role, route) {
  page.removeAllListeners()
  page.on('console', m => m.type() === 'error' && record(role, route, 'console', m.text()))
  page.on('pageerror', e => record(role, route, 'exception', e))
  page.on('requestfailed', r => {
    const why = r.failure()?.errorText ?? ''
    if (TRANSIENT.test(why)) return // the machine's network, not the app
    record(role, route, 'request-failed', `${r.url()} ${why}`)
  })
  page.on('response', r => r.status() >= 400 && record(role, route, `http-${r.status()}`, r.url()))
}

// `networkidle` is not usable here: a cold free-tier API can hold a request
// open for the better part of a minute, and fonts keep a connection warm. Wait
// for the element the step actually needs instead.
// Injected into every page so the predicates above can use it.
const TXT_HELPER = `window.txt = () => document.body.innerText.replace(/\\s+/g, ' ').toLowerCase()`

async function signIn(page, email) {
  await goto(page, `${BASE}/login`)
  await page.waitForSelector('input[type=email]', { timeout: 60000 })
  await page.fill('input[type=email]', email)
  await page.fill('input[type=password]', PASSWORD)
  await page.locator('form button[type=submit], form button').first().click()
  await page.waitForSelector('header', { timeout: 90000 })
}

// Wake the API first: on a free tier the first request can take 50 seconds,
// and every route would otherwise pay for it once.
try {
  const api = process.env.SMOKE_API
  if (api) {
    process.stdout.write('warming the API... ')
    const t = Date.now()
    await fetch(`${api}/healthz`).catch(() => {})
    console.log(`${Date.now() - t}ms`)
  }
} catch {}

const browser = await chromium.launch({ channel: 'chrome' })
let checks = 0

for (const [role, routes] of Object.entries(ROUTES)) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  await page.addInitScript(TXT_HELPER)
  listen(page, role, 'login')

  if (LOGIN[role]) {
    await signIn(page, LOGIN[role])
    if (!(await page.locator('header').count())) {
      record(role, 'login', 'login-failed', `could not sign in as ${LOGIN[role]}`)
    }
  }

  for (const [route, ready] of routes) {
    listen(page, role, route)
    checks++
    try {
      await goto(page, BASE + route)
      // Poll the assertion itself. A cold API can take 30s to answer, and a
      // fixed sleep would either be flaky or pointlessly slow.
      await page.waitForFunction(
        `(${String(ready)})()`,
        null,
        { timeout: 60000, polling: 500 },
      )
    } catch (e) {
      const text = await page.locator('body').innerText().catch(() => '')
      record(role, route, /Timeout/.test(String(e)) ? 'assertion-never-true' : 'nav-failed',
        `${String(e).split('\n')[0]} | page: ${text.replace(/\s+/g, ' ').slice(0, 180)}`)
    }
  }
  await ctx.close()
}
await browser.close()

const unique = [...new Map(problems.map(p => [`${p.role}|${p.route}|${p.kind}|${p.text.slice(0, 90)}`, p])).values()]
console.log(`\n${checks} checks across ${Object.keys(ROUTES).length} roles against ${BASE}`)
if (!unique.length) {
  console.log('PASS — no console errors, no failed requests, all content present')
  process.exit(0)
}
console.log(`FAIL — ${unique.length} problem(s):\n`)
for (const p of unique) console.log(`  [${p.role}] ${p.route} (${p.kind})\n    ${p.text}\n`)
process.exit(1)
