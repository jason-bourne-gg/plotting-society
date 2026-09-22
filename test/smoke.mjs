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
import { chromium } from 'playwright'

const BASE = (process.argv[2] ?? 'http://localhost:5173').replace(/\/$/, '')
const PASSWORD = process.env.SMOKE_PASSWORD ?? 'sandesh-demo-2026'

const LOGIN = {
  owner: 'rohit.deshmukh@example.com',
  builder: 'office@shivrudragroup.in',
}

// expect: a substring the page must contain, or a function for a richer check.
const ROUTES = {
  guest: [
    ['/explore', 'Sandesh Nagari 7'],
    ['/explore', '823'],                       // the highlights array rendered
    ['/explore', 'Club House'],                // the amenities array rendered
  ],
  owner: [
    ['/', 'Layout map', async p => {
      const n = await p.locator('svg polygon').count()
      return n > 100 ? null : `only ${n} plot polygons drawn, expected 823`
    }],
    ['/my-plot', 'One-time maintenance'],
    ['/my-plot', 'sq ft ×'],                   // the bill shows its working
    ['/updates', 'Site progress'],
    ['/fund', 'Closing balance'],
    ['/queries', 'My queries'],
    ['/queries/new', 'Raise a query'],
  ],
  builder: [
    ['/admin', 'Past SLA'],
    ['/admin/inbox', 'Query inbox'],
    ['/admin/leads', 'Leads'],
    ['/admin/plots', 'Plots'],
    ['/admin/updates', 'Post a site update'],
    ['/admin/fund', 'Fund ledger'],
    ['/admin/maintenance', '1,540 sq ft'],     // the reference plot quote
  ],
}

const problems = []
const record = (role, route, kind, text) =>
  problems.push({ role, route, kind, text: String(text).replace(/\s+/g, ' ').slice(0, 260) })

function listen(page, role, route) {
  page.removeAllListeners()
  page.on('console', m => m.type() === 'error' && record(role, route, 'console', m.text()))
  page.on('pageerror', e => record(role, route, 'exception', e))
  page.on('requestfailed', r => record(role, route, 'request-failed', `${r.url()} ${r.failure()?.errorText}`))
  page.on('response', r => r.status() >= 400 && record(role, route, `http-${r.status()}`, r.url()))
}

async function signIn(page, email) {
  await page.goto(`${BASE}/login`, { waitUntil: 'networkidle', timeout: 60000 })
  await page.fill('input[type=email]', email)
  await page.fill('input[type=password]', PASSWORD)
  await page.click('button[type=submit]')
  await page.waitForTimeout(4000)
}

const browser = await chromium.launch({ channel: 'chrome' })
let checks = 0

for (const [role, routes] of Object.entries(ROUTES)) {
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } })
  const page = await ctx.newPage()
  listen(page, role, 'login')

  if (LOGIN[role]) {
    await signIn(page, LOGIN[role])
    if (!(await page.locator('header').count())) {
      record(role, 'login', 'login-failed', `could not sign in as ${LOGIN[role]}`)
    }
  }

  for (const [route, expect, extra] of routes) {
    listen(page, role, route)
    checks++
    try {
      await page.goto(BASE + route, { waitUntil: 'networkidle', timeout: 60000 })
      await page.waitForTimeout(2000)

      const text = (await page.locator('body').innerText()).replace(/\s+/g, ' ')
      if (!text.includes(expect)) {
        record(role, route, 'missing-content', `expected "${expect}", page shows: ${text.slice(0, 160)}`)
      }
      if (/Could not load|Something went wrong|does not exist/i.test(text)) {
        record(role, route, 'error-shown', text.slice(0, 160))
      }
      if (extra) {
        const failure = await extra(page)
        if (failure) record(role, route, 'assertion', failure)
      }
    } catch (e) {
      record(role, route, 'nav-failed', e)
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
