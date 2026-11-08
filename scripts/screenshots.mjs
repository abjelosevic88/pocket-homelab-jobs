// README screenshots and a browser smoke test, against a demo instance (DEMO_DATA=true seeds a ten-week job search):
//
//   DATA_DIR=/tmp/jobs-demo DEMO_DATA=true ADMIN_EMAIL=demo@example.com ADMIN_PASSWORD=demodemo1 ADMIN_NAME=Mirela PORT=18259 ./pocket-jobs
//   CDP=http://127.0.0.1:9340 BASE=http://127.0.0.1:18259 node scripts/screenshots.mjs   # any Chrome with remote debugging
//   SHOTS=0 … node scripts/screenshots.mjs                                                # checks only
// playwright-core comes from web/node_modules, or set PW_FROM to a package.json next to an install of it.
import { createRequire } from 'node:module'
import assert from 'node:assert/strict'

const require = createRequire(process.env.PW_FROM || new URL('../web/package.json', import.meta.url))
let pw
try { pw = require('playwright') } catch { pw = require('playwright-core') }
const { chromium } = pw
const base = process.env.BASE || 'http://127.0.0.1:18259'
const shots = process.env.SHOTS !== '0'
const only = process.env.ONLY || ''
const dir = process.env.OUT || new URL('../docs/screenshots/', import.meta.url).pathname
const browser = process.env.CDP ? await chromium.connectOverCDP(process.env.CDP) : await chromium.launch()
const errors = []

async function session({ phone = false, dark = false } = {}) {
  const ctx = await browser.newContext({
    viewport: phone ? { width: 390, height: 844 } : { width: 1440, height: 900 },
    deviceScaleFactor: phone ? 3 : 2, isMobile: phone, hasTouch: phone, colorScheme: dark ? 'dark' : 'light',
    userAgent: phone ? 'Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Mobile/15E148 Safari/604.1' : undefined,
  })
  const page = await ctx.newPage()
  page.on('pageerror', e => errors.push(`pageerror: ${e}`))
  page.on('console', m => { if (m.type() === 'error' && !/Failed to load resource|Content Security Policy/.test(m.text())) errors.push(m.text()) })
  await page.goto(base + '/login')
  const res = await page.evaluate(() => fetch('/api/v1/auth/login', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ email: 'demo@example.com', password: 'demodemo1' }) }).then(r => r.status))
  assert.equal(res, 200, 'demo sign-in')
  return { ctx, page }
}
async function go(page, path, wait = 1500) {
  await page.goto(base + path, { waitUntil: 'networkidle' })
  // headless Chrome freezes CSS fade-ins: show everything at once
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' }).catch(() => {})
  await page.evaluate(() => document.fonts.ready)
  await page.waitForTimeout(wait)
}
async function shot(page, name) { if (shots && (!only || only.split(',').includes(name))) await page.screenshot({ path: dir + name + '.png' }) }
const api = async (page, path) => page.evaluate(p => fetch('/api/v1' + p).then(r => r.json()), path)
// the job with an offer and interviews: the richest page
const jobID = async page => (await api(page, '/jobs?status=open&sort=fit&q=Northwind')).items[0].id

// ---- desktop ----
{
  const { ctx, page } = await session()
  await go(page, '/', 1500)
  assert.ok((await page.content()).includes('Applied this week'), 'Today shows the week')
  await shot(page, 'desktop-today')
  await go(page, '/board', 1500)
  assert.ok((await page.content()).includes('Interviewing'), 'the board has its stages')
  await shot(page, 'desktop-board')
  const id = await jobID(page)
  await go(page, `/jobs/${id}`, 2000)
  assert.ok((await page.content()).includes('Timeline'), 'a job shows its timeline')
  await shot(page, 'desktop-job')
  await go(page, '/interviews', 1200)
  await shot(page, 'desktop-interviews')
  await go(page, '/offers', 1200)
  assert.ok((await page.content()).includes('Pays the most'), 'offers side by side')
  await shot(page, 'desktop-offers')
  await go(page, '/cvs', 1200)
  await shot(page, 'desktop-cvs')
  await go(page, '/jobs', 1200)
  await shot(page, 'desktop-jobs')
  await go(page, '/companies', 1200)
  await shot(page, 'desktop-companies')
  await go(page, '/reports', 1500)
  await shot(page, 'desktop-reports')
  await go(page, '/settings/stages', 1200)
  await shot(page, 'desktop-settings-stages')
  await go(page, '/settings/apps', 1500)
  await shot(page, 'desktop-settings-apps')
  await go(page, '/board', 1000)
  await page.keyboard.press('n')
  await page.waitForTimeout(600)
  await shot(page, 'desktop-new')
  await ctx.close()
}

// ---- phone ----
{
  const { ctx, page } = await session({ phone: true })
  await go(page, '/', 1500)
  await shot(page, 'phone-today')
  await go(page, '/board', 1500)
  await shot(page, 'phone-board')
  const id = await jobID(page)
  await go(page, `/jobs/${id}`, 2000)
  await shot(page, 'phone-job')
  await go(page, '/offers', 1200)
  await shot(page, 'phone-offers')
  await go(page, '/reports', 1500)
  await shot(page, 'phone-reports')
  await ctx.close()
}

// ---- dark ----
{
  const { ctx, page } = await session({ dark: true })
  await go(page, '/board', 1500)
  await shot(page, 'desktop-board-dark')
  const id = await jobID(page)
  await go(page, `/jobs/${id}`, 2000)
  await shot(page, 'desktop-job-dark')
  await ctx.close()
}

await browser.close()
if (errors.length) { console.error(errors.join('\n')); process.exit(1) }
console.log('ok')
