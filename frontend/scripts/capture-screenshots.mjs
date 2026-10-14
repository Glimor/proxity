// Render the actual Vue UI with deterministic, non-sensitive demo data.
// From frontend/: node scripts/capture-screenshots.mjs
import { mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { chromium } from 'playwright'

const output = fileURLToPath(new URL('../../docs/screenshots/', import.meta.url))
const server = await createServer({ server: { host: '127.0.0.1', port: 0 } })
let browser
try {
  await mkdir(output, { recursive: true })
  await server.listen()
  browser = await chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || undefined })
  const page = await browser.newPage({ viewport: { width: 1180, height: 780 }, deviceScaleFactor: 1, colorScheme: 'dark' })
  await page.mouse.move(1170, 770)
  const errors = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()) })
  await page.addInitScript(() => {
    const profiles = [
      { ID: 'work', Name: 'Work proxy', Host: 'work.proxy.example', Port: 1080, Username: 'demo', Enabled: true },
      { ID: 'personal', Name: 'Personal proxy', Host: 'personal.proxy.example', Port: 1080, Username: '', Enabled: true },
    ]
    const rules = [
      { ID: 'browser', Name: 'Browser', Kind: 'exe', Path: 'C:\\Apps\\Browser\\browser.exe', Action: 'proxy', ProxyID: 'personal', KillSwitch: true, Enabled: true },
      { ID: 'work-tools', Name: 'Work tools', Kind: 'folder', Path: 'C:\\Apps\\Work', Action: 'proxy', ProxyID: 'work', KillSwitch: true, Enabled: true, IncludeSubdirs: true },
      { ID: 'downloads', Name: 'Downloads', Kind: 'folder', Path: 'C:\\Apps\\Downloads', Action: 'proxy', ProxyID: 'personal', KillSwitch: true, Enabled: true },
      { ID: 'local', Name: 'Local tools', Kind: 'folder', Path: 'C:\\Apps\\Local', Action: 'direct', Enabled: true },
      { ID: 'blocked', Name: 'Offline app', Kind: 'exe', Path: 'C:\\Apps\\Offline\\offline.exe', Action: 'block', Enabled: true },
    ]
    const history = Array.from({ length: 90 }, (_, i) => ({
      total_down_bps: Math.round((1.5 + Math.sin(i / 9) * 0.4 + Math.cos(i / 3) * 0.15) * 1048576),
      total_up_bps: Math.round((0.22 + Math.sin(i / 7) * 0.08) * 1048576),
      active_conns: 24, blocked_count: 12, per_exe: {}, per_proxy: {},
    }))
    const settings = {
      language: 'en', theme: 'dark', kill_switch_global: true, lan_access: true,
      secure_dns: false, dns_upstream: '', dns_via_proxy_id: '', notifications_enabled: true,
      notify_blocked: true, notify_proxy_events: true, auto_start_service: true,
      auto_start_gui: false, start_minimized: false, default_action: 'direct', log_level: 'info',
    }
    window.__demoSettings = settings
    const handlers = {
      GetGuiPrefs: () => ({ language: 'en', theme: 'dark', wizard_done: true }),
      Connected: () => true, GetSettings: () => settings,
      EngineStatus: () => ({ running: true, capture_alive: true, listeners: 2 }),
      GetSnapshot: () => history.at(-1), GetHistory: () => history,
      GetProfiles: () => profiles, GetRules: () => rules,
      GetProfileStatuses: () => profiles.map((p, i) => ({ ProfileID: p.ID, State: 2, LatencyMS: 28 + i * 14, PublicIP: `203.0.113.${10 + i}`, LastError: '' })),
      GetConnections: () => [], LastSession: () => null,
      GetExeIcon: () => '', GetBlocked: () => [],
    }
    window.go = { main: { App: Object.fromEntries(Object.entries(handlers).map(([name, fn]) => [name, async (...args) => fn(...args)])) } }
    window.runtime = { EventsOn: () => () => {} }
  })
  await page.goto(server.resolvedUrls.local[0])
  await page.getByRole('heading', { name: 'Dashboard', exact: true }).waitFor()
  await page.locator('aside img').evaluate((img) => img.decode())
  await page.locator('canvas').waitFor()
  await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))))
  await page.screenshot({ path: `${output}/dashboard.png`, animations: 'disabled' })
  for (const [name, file] of [['Applications', 'applications'], ['Proxies', 'proxies']]) {
    await page.getByRole('link', { name: new RegExp(`${name}$`) }).click()
    await page.getByRole('heading', { name, exact: true }).waitFor()
    await page.getByText(name === 'Applications' ? 'Work tools' : 'work.proxy.example:1080', { exact: true }).waitFor()
    await page.mouse.move(1170, 770)
    await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))))
    await page.screenshot({ path: `${output}/${file}.png`, animations: 'disabled' })
  }
  await page.getByRole('link', { name: /Dashboard$/ }).click()
  await page.getByRole('heading', { name: 'Dashboard', exact: true }).waitFor()
  await page.evaluate(async () => {
    window.__demoSettings.theme = 'light'
    const { useApp } = await import('/src/stores/app.ts')
    await useApp().refreshSettings()
  })
  await page.mouse.move(1170, 770)
  await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))))
  await page.screenshot({ path: `${output}/dashboard-light.png`, animations: 'disabled' })
  if (errors.length) throw new Error(errors.join('\n'))
  console.log(`Screenshots saved to ${output}`)
} finally {
  await browser?.close()
  await server.close()
}
