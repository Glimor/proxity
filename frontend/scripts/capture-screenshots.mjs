// Render the actual Vue UI with deterministic, non-sensitive demo data.
// From frontend/: node scripts/capture-screenshots.mjs
import { mkdir, mkdtemp, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { createServer } from 'vite'
import { chromium } from 'playwright'

const output = fileURLToPath(new URL('../../docs/screenshots/', import.meta.url))
const cacheDir = await mkdtemp(join(tmpdir(), 'proxity-screenshots-vite-'))
const viewport = { width: 1280, height: 850 }
const browserPath = 'C:\\Apps\\Browser\\browser.exe'
let server, browser
try {
  server = await createServer({ cacheDir, server: { host: '127.0.0.1', port: 0, hmr: false } })
  await server.listen()
  const baseURL = server.resolvedUrls.local[0]
  const origin = new URL(baseURL).origin
  browser = await chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || undefined })
  const page = await browser.newPage({
    viewport, deviceScaleFactor: 1, colorScheme: 'dark', reducedMotion: 'reduce', serviceWorkers: 'block',
  })
  page.setDefaultTimeout(15_000)
  const errors = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => { if (message.type() === 'error') errors.push(message.text()) })
  await page.route('**/*', (route) => {
    const url = new URL(route.request().url())
    if (url.origin === origin) return route.continue()
    errors.push(`Unexpected screenshot page request: ${url.origin}`)
    return route.abort('blockedbyclient')
  })
  await page.addInitScript((browserPath) => {
    const personalProxy = { ID: 'personal', Protocol: 'socks5', Name: 'Personal proxy', Host: 'personal.proxy.example', Port: 1080, Username: '', Enabled: true }
    const browserRule = { ID: 'browser', Name: 'Browser', Kind: 'exe', Path: browserPath, Action: 'proxy', ProxyID: personalProxy.ID, KillSwitch: true, Enabled: true }
    const profiles = [
      { ID: 'work', Protocol: 'https', Name: 'Work proxy', Host: 'work.proxy.example', Port: 1080, Username: '', Enabled: true },
      personalProxy,
    ]
    const rules = [
      browserRule,
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
      language: 'en', theme: 'system', kill_switch_global: true, lan_access: true,
      secure_dns: false, dns_upstream: '', dns_via_proxy_id: '', notifications_enabled: true,
      notify_blocked: true, notify_proxy_events: true, auto_start_service: true,
      auto_start_gui: false, start_minimized: false, default_action: 'direct', log_level: 'info',
    }
    const engine = { running: true, capture_alive: true, listeners: profiles.length }
    const handlers = {
      GetGuiPrefs: () => ({ language: settings.language, theme: settings.theme, wizard_done: true, start_minimized: settings.start_minimized }),
      Connected: () => true, GetSettings: () => ({ ...settings }),
      GuiAutoStart: () => settings.auto_start_gui,
      GuiVersion: () => 'dev', ServiceVersion: () => 'dev',
      EngineStatus: () => engine,
      GetSnapshot: () => history.at(-1), GetHistory: () => history,
      GetProfiles: () => profiles, GetRules: () => rules,
      GetProfileStatuses: () => profiles.map((p, i) => ({ ProfileID: p.ID, State: 2, LatencyMS: 28 + i * 14, PublicIP: `203.0.113.${10 + i}`, LastError: '' })),
      GetConnections: () => [], LastSession: () => null,
      GetExeIcon: () => '', GetBlocked: () => [],
      PickExeFile: () => browserRule.Path,
      PreviewRoute: (path) => {
        if (path !== browserRule.Path) throw new RangeError('No screenshot fixture for this executable')
        return {
          exe_path: path, action: browserRule.Action, reason: 'exact_exe',
          rule: { name: browserRule.Name, path: browserRule.Path, kind: browserRule.Kind },
          proxy: { name: personalProxy.Name, availability: 'connected', fallback_action: 'block' },
          engine, exe_policy: 'kill_switch', kill_switch_global: settings.kill_switch_global,
          lan_access: settings.lan_access, secure_dns: settings.secure_dns, service_executable: false,
        }
      },
    }
    window.go = { main: { App: Object.fromEntries(Object.entries(handlers).map(([name, fn]) => [name, async (...args) => fn(...args)])) } }
    window.runtime = { EventsOn: () => () => {} }
  }, browserPath)

  const frames = new Map()
  const heading = (name) => page.getByRole('heading', { name, exact: true })
  async function navigate(name) {
    await page.getByRole('navigation', { name: 'Main navigation' }).getByRole('link', { name, exact: true }).click()
    await heading(name).waitFor()
  }
  async function capture(file) {
    await page.mouse.move(viewport.width - 10, viewport.height - 10)
    await page.evaluate(async () => {
      await document.fonts.ready
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
    })
    frames.set(file, await page.screenshot({ animations: 'disabled' }))
  }

  await page.goto(baseURL)
  await heading('Routing is active').waitFor()
  await page.locator('aside img').evaluate((img) => img.decode())
  await page.locator('html.dark canvas[aria-label="Recent download and upload rates"]').waitFor()
  await capture('dashboard')
  await page.emulateMedia({ colorScheme: 'light' })
  await page.locator('html:not(.dark) canvas[aria-label="Recent download and upload rates"]').waitFor()
  await capture('dashboard-light')
  await page.emulateMedia({ colorScheme: 'dark' })
  await page.locator('html.dark').waitFor()

  await navigate('Applications')
  const rulesTable = page.getByRole('table', { name: 'Saved rules', exact: true })
  await rulesTable.getByRole('row', { name: /Browser.*Personal proxy/ }).waitFor()
  await rulesTable.getByText('Offline app', { exact: true }).waitFor()
  await capture('applications')

  await page.getByRole('button', { name: 'Preview route', exact: true }).click()
  const preview = page.getByRole('dialog', { name: 'Preview route', exact: true })
  await preview.locator(':scope > p').and(preview.getByText(browserPath, { exact: true })).waitFor()
  for (const text of ['Saved route', 'Exact executable match', 'Personal proxy', 'Block if the proxy is unavailable while capture is active.']) {
    await preview.getByText(text, { exact: true }).waitFor()
  }
  const done = preview.getByRole('button', { name: 'Done', exact: true })
  await done.waitFor()
  await capture('route-preview')
  await done.click()
  await preview.waitFor({ state: 'hidden' })

  await navigate('Proxies')
  await page.getByText('HTTPS · work.proxy.example:1080', { exact: true }).waitFor()
  await page.getByText('SOCKS5 · personal.proxy.example:1080', { exact: true }).waitFor()
  await page.getByText(/203\.0\.113\.11/).waitFor()
  await capture('proxies')

  await navigate('Settings')
  const startup = page.getByRole('region', { name: 'Startup', exact: true })
  await startup.getByRole('switch', { name: 'Apply routing rules when Windows starts', exact: true, checked: true }).waitFor()
  await startup.getByRole('switch', { name: 'Open Proxity at sign-in', exact: true, checked: false }).waitFor()
  await capture('startup-settings')

  if (errors.length) throw new AggregateError(errors.map((message) => new Error(message)), 'Screenshot page errors')
  await mkdir(output, { recursive: true })
  await Promise.all([...frames].map(([file, png]) => writeFile(join(output, `${file}.png`), png)))
  console.log(`Screenshots saved to ${output}`)
} finally {
  try {
    await browser?.close()
  } finally {
    try {
      await server?.close()
    } finally {
      await rm(cacheDir, { recursive: true, force: true })
    }
  }
}
