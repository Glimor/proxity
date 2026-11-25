import assert from 'node:assert/strict'
import { after, before, test } from 'node:test'
import { createServer } from 'vite'
import { chromium } from 'playwright'

let server, browser, baseURL
before(async () => {
  server = await createServer({ server: { host: '127.0.0.1', port: 0 } })
  await server.listen()
  baseURL = server.resolvedUrls.local[0]
  browser = await chromium.launch({ channel: process.env.PLAYWRIGHT_CHANNEL || undefined })
})
after(async () => {
  await browser?.close()
  await server?.close()
})

async function openGUI(options = {}) {
  const page = await browser.newPage()
  page.setDefaultTimeout(8000)
  const errors = []
  page.on('pageerror', (error) => errors.push(error.message))
  page.on('console', (message) => {
    if (message.type() === 'error') errors.push(message.text())
  })
  // The native Wails shell supplies the window icon; a browser test has none.
  await page.route('**/favicon.ico', (route) => route.fulfill({ status: 204 }))
  await page.addInitScript((options) => {
    const settings = {
      language: 'en', theme: 'system', kill_switch_global: false, lan_access: true,
      secure_dns: false, dns_upstream: '', dns_via_proxy_id: '', notifications_enabled: true,
      notify_blocked: true, notify_proxy_events: true, auto_start_service: true,
      auto_start_gui: false, start_minimized: false, default_action: options.defaultAction ?? 'direct', log_level: 'info',
    }
    const state = window.__test = {
      calls: [], events: {}, profiles: options.profiles || [], rules: options.rules || [],
      failRule: false, failPrefs: false, connected: options.connected !== false,
      settings, prefs: { language: 'en', theme: 'system', wizard_done: !!options.wizardDone, start_minimized: false },
    }
    const list = (value) => value.length ? value : null
    const record = (method, fn) => async (...args) => {
      state.calls.push({ method, args })
      return fn(...args)
    }
    const toProfile = (input, id) => ({
      ID: id, Protocol: input.protocol || 'socks5', Name: input.name, Host: input.host, Port: input.port, Username: input.username,
      Enabled: input.enabled, TimeoutMS: input.timeout_ms,
      RetryIntervalMS: input.retry_interval_ms, MaxRetries: input.max_retries,
    })
    const methods = Object.fromEntries(Object.entries({
      GetGuiPrefs: () => state.prefs,
      SaveGuiPrefs: (prefs) => {
        if (state.failPrefs) throw new Error('Cannot save preferences')
        state.prefs = prefs
      },
      Connected: () => state.connected,
      GetSettings: () => ({ ...state.settings }),
      UpdateSettings: (value) => state.settings = value,
      EngineStatus: () => options.engine ?? { running: false, capture_alive: false, listeners: 0 },
      GetSnapshot: () => ({ total_up_bps: 0, total_down_bps: 0, active_conns: 0, per_exe: {}, per_proxy: {} }),
      GetHistory: () => null, GetConnections: () => null, GetProfileStatuses: () => null,
      GetProfiles: () => list(state.profiles), GetRules: () => list(state.rules),
      GetExeStats: () => null, GetProxyStats: () => null, GetBlocked: () => null,
      ListProcesses: () => null, ExportProfiles: () => null, LastSession: () => null,
      GuiAutoStart: () => false, GuiVersion: () => 'test', ServiceVersion: () => 'test',
      ServiceStatus: () => ({ binary_found: false, installed: true, running: true }),
      CreateProfile: (input) => {
        const profile = toProfile(input, `proxy-${state.profiles.length + 1}`)
        state.profiles.push(profile)
        return profile
      },
      UpdateProfile: (id, input) => {
        const profile = toProfile(input, id)
        state.profiles = state.profiles.map((p) => p.ID === id ? profile : p)
        return profile
      },
      TestProfile: async () => {
        if (state.testDelay) await new Promise((resolve) => setTimeout(resolve, state.testDelay))
        return { latency_ms: 12, public_ip: '203.0.113.1' }
      },
      PickExeFile: () => options.picks ? options.picks.shift() : 'C:\\Apps\\app.exe', GetExeIcon: () => '',
      PreviewRoute: async (path) => {
        if (state.holdPreview) {
          state.holdPreview = false
          await new Promise((resolve) => { state.releasePreview = resolve })
        }
        return options.previews[path]
      },
      CreateRule: (input) => {
        if (state.failRule) throw new Error('Rule could not be saved')
        const rule = { ID: 'rule-1', Name: input.name, Path: input.path, Kind: input.kind,
          ProxyID: input.proxy_id, Action: input.action, Enabled: input.enabled }
        state.rules.push(rule)
        return { rule }
      },
      UpdateRule: (id, input) => {
        const rule = { ID: id, Name: input.name, Kind: input.kind, Path: input.path,
          Action: input.action, ProxyID: input.proxy_id, Enabled: input.enabled,
          KillSwitch: input.kill_switch, IncludeSubdirs: input.include_subdirs, Priority: input.priority }
        state.rules = state.rules.map((r) => r.ID === id ? rule : r)
        return { rule, conflicts: [{ ID: 'other', Name: 'Existing rule' }] }
      },
      UninstallService: () => { throw new Error('Autostart must not uninstall the service') },
    }).map(([method, fn]) => [method, record(method, fn)]))
    // A call to an unconfigured binding must be observable even when the UI catches its rejection.
    window.go = { main: { App: new Proxy(methods, { get: (target, method) => target[method] ??
      record(method, () => { throw new Error(`Unexpected Wails call: ${method}`) }) }) } }
    window.runtime = {
      EventsOn: (name, cb) => {
        const callbacks = state.events[name] ||= []
        callbacks.push(cb)
        return () => { state.events[name] = callbacks.filter((fn) => fn !== cb) }
      },
    }
  }, options)
  await page.goto(baseURL)
  await page.locator('h1').waitFor()
  return { page, errors }
}

test('skip first-run setup and open every menu with null lists', async () => {
  const { page, errors } = await openGUI()
  try {
    assert.equal(await page.locator('h1').innerText(), 'Proxity Setup')
    await page.getByRole('button', { name: 'Skip', exact: true }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Skip', exact: true }).click()
    await page.getByRole('heading', { name: 'Dashboard', exact: true }).waitFor()
    for (const name of ['Proxies', 'Applications', 'Connections', 'Statistics', 'Blocked', 'Settings', 'Dashboard']) {
      await page.getByRole('link', { name: new RegExp(`${name}$`) }).click()
      await page.getByRole('heading', { name, exact: true, level: 1 }).waitFor()
    }
    const lists = await page.evaluate(async () => {
      const { api } = await import('/src/api.ts')
      return Promise.all([api.getProfiles(), api.getRules(), api.getHistory(), api.getConnections(),
        api.getProfileStatuses(), api.getExeStats('', ''), api.getProxyStats('', ''),
        api.getBlocked(200), api.listProcesses(), api.exportProfiles(false)])
    })
    assert.deepEqual(lists, Array.from({ length: 10 }, () => []))
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})

test('wizard persists SOCKS5 credentials, updates tested input and selects the created proxy', async () => {
  const { page, errors } = await openGUI()
  try {
    await page.getByRole('button', { name: 'Next', exact: true }).click()
    const inputs = page.locator('input')
    await inputs.nth(0).fill('Authenticated SOCKS5')
    await inputs.nth(1).fill('127.0.0.1')
    await inputs.nth(2).fill('1080')
    await inputs.nth(3).fill('alice')
    await inputs.nth(4).fill('first-secret')
    await page.getByRole('button', { name: 'Test Connection', exact: true }).click()
    await page.getByText('Connected — 12 ms, exit IP: 203.0.113.1', { exact: true }).waitFor()
    await inputs.nth(4).fill('corrected-secret')
    await page.getByRole('button', { name: 'Test Connection', exact: true }).click()
    await page.getByText('Connected — 12 ms, exit IP: 203.0.113.1', { exact: true }).waitFor()
    await page.getByRole('button', { name: 'Next', exact: true }).click()
    assert.equal(await page.locator('select').inputValue(), 'proxy-1')
    await page.getByRole('button', { name: 'Pick a file (.exe)', exact: true }).click()
    await page.getByText('C:\\Apps\\app.exe', { exact: true }).waitFor()
    await page.evaluate(() => { window.__test.failRule = true })
    await page.getByRole('button', { name: 'Finish', exact: true }).click()
    await page.getByText('Rule could not be saved', { exact: true }).waitFor()
    assert.equal(await page.locator('select').inputValue(), 'proxy-1')
    await page.evaluate(() => { window.__test.failRule = false })
    await page.getByRole('button', { name: 'Finish', exact: true }).click()
    await page.getByRole('heading', { name: "3. You're all set!", exact: true }).waitFor()
    await page.getByRole('button', { name: 'Finish', exact: true }).click()
    await page.getByRole('heading', { name: 'Dashboard', exact: true }).waitFor()
    const state = await page.evaluate(() => window.__test)
    const creations = state.calls.filter((c) => c.method === 'CreateProfile')
    const updates = state.calls.filter((c) => c.method === 'UpdateProfile')
    assert.equal(creations.length, 1)
    assert.equal(creations[0].args[0].username, 'alice')
    assert.equal(creations[0].args[0].password, 'first-secret')
    assert.equal(updates.length, 1)
    assert.equal(updates[0].args[1].password, 'corrected-secret')
    assert.equal(state.rules[0].ProxyID, 'proxy-1')
    assert.equal(state.prefs.wizard_done, true)
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})

for (const protocol of ['http', 'https', 'socks4', 'socks5']) {
  test(`wizard saves and tests ${protocol} profiles`, async () => {
    const { page, errors } = await openGUI()
    try {
      await page.getByRole('button', { name: 'Next', exact: true }).click()
      await page.getByLabel('Proxy type', { exact: true }).selectOption(protocol)
      await page.locator('input').nth(1).fill('proxy.example.com')
      await page.locator('input').nth(3).fill('alice')
      if (protocol === 'socks4') {
        assert.equal(await page.locator('input[type=password]').count(), 0)
        await page.getByText('User ID (optional)', { exact: true }).waitFor()
      } else {
        await page.locator('input[type=password]').fill('secret')
      }
      await page.getByRole('button', { name: 'Test Connection', exact: true }).click()
      await page.getByText('Connected — 12 ms, exit IP: 203.0.113.1', { exact: true }).waitFor()
      const state = await page.evaluate(() => window.__test)
      assert.equal(state.profiles[0].Protocol, protocol)
      assert.equal(state.calls.find((c) => c.method === 'CreateProfile').args[0].password, protocol === 'socks4' ? '' : 'secret')
      assert.deepEqual(errors, [])
    } finally { await page.close() }
  })

  test(`proxy editor creates, displays and edits ${protocol} profiles`, async () => {
    const { page, errors } = await openGUI({ wizardDone: true })
    try {
      await page.getByRole('link', { name: /Proxies$/ }).click()
      await page.getByRole('button', { name: /Add Proxy/ }).first().click()
      let modal = page.getByRole('dialog')
      await modal.getByLabel('Proxy type', { exact: true }).selectOption(protocol)
      await modal.locator('input').nth(1).fill('proxy.example.com')
      await modal.locator('input').nth(3).fill('alice')
      if (protocol !== 'socks4') await modal.locator('input[type=password]').fill('secret')
      else assert.equal(await modal.locator('input[type=password]').count(), 0)
      await modal.getByRole('button', { name: 'Save', exact: true }).click()
      await page.getByText(`${protocol.toUpperCase()} · proxy.example.com:1080`, { exact: true }).waitFor()
      await page.getByRole('button', { name: 'Edit', exact: true }).click()
      modal = page.getByRole('dialog')
      assert.equal(await modal.getByLabel('Proxy type', { exact: true }).inputValue(), protocol)
      const next = protocol === 'socks4' ? 'https' : 'socks4'
      await modal.getByLabel('Proxy type', { exact: true }).selectOption(next)
      await modal.getByRole('button', { name: 'Test Connection', exact: true }).click()
      await modal.getByText('Connected — 12 ms, exit IP: 203.0.113.1', { exact: true }).waitFor()
      await modal.getByRole('button', { name: 'Save', exact: true }).click()
      await page.getByText(`${next.toUpperCase()} · proxy.example.com:1080`, { exact: true }).waitFor()
      assert.equal(await page.evaluate(() => window.__test.profiles[0].Protocol), next)
      assert.deepEqual(errors, [])
    } finally { await page.close() }
  })
}

test('preview shows the saved exact rule or default for the latest selected executable without writing', async () => {
  const browserPath = String.raw`C:\Apps\Browser\browser.exe`
  const otherPath = String.raw`C:\Apps\Other\other.exe`
  const rule = { ID: 'r-browser', Name: 'Browser route', Kind: 'exe', Path: browserPath, Action: 'proxy',
    Enabled: true, Priority: 0, ProxyID: 'p-office', KillSwitch: false }
  const profile = { ID: rule.ProxyID, Name: 'Office proxy', Enabled: true }
  const context = { engine: { running: true, capture_alive: true, listeners: 2 }, exe_policy: 'none',
    kill_switch_global: false, lan_access: true, secure_dns: false, service_executable: false }
  const { page, errors } = await openGUI({ wizardDone: true, rules: [rule], profiles: [profile],
    engine: context.engine, defaultAction: 'block',
    picks: ['', browserPath, browserPath, otherPath], previews: {
      [browserPath]: { ...context, exe_path: browserPath, exe_policy: 'udp_only', action: 'proxy', reason: 'exact_exe',
        rule: { name: rule.Name, path: browserPath, kind: 'exe' },
        proxy: { name: profile.Name, availability: 'connected', fallback_action: 'direct' } },
      [otherPath]: { ...context, exe_path: otherPath, action: 'direct', reason: 'default', saved_default_action: 'block' },
    } })
  try {
    await page.getByRole('link', { name: 'Applications', exact: true }).click()
    await page.getByRole('switch', { name: 'Kill switch for Browser route', checked: false }).waitFor()
    const start = await page.evaluate(() => window.__test.calls.length)
    const trigger = page.getByRole('button', { name: 'Preview route', exact: true })
    const dialog = page.getByRole('dialog', { name: 'Preview route', exact: true })
    const repick = dialog.getByRole('button', { name: 'Choose another file', exact: true })
    const field = (name) => dialog.getByText(name, { exact: true }).locator('..')
    const calls = () => page.evaluate((start) => window.__test.calls.slice(start), start)
    await trigger.click()
    assert.deepEqual(await calls(), [{ method: 'PickExeFile', args: [] }])
    assert.equal(await dialog.isVisible(), false)

    await trigger.click()
    await dialog.locator(':scope > p').and(dialog.getByText(browserPath, { exact: true })).waitFor()
    await field('Saved route').getByText('Proxy', { exact: true }).waitFor()
    await field('Winning match').getByText('Exact executable match', { exact: true }).waitFor()
    await field('Winning match').getByText(/Browser route · Application/).waitFor()
    await field('Proxy profile').getByText('Office proxy', { exact: true }).waitFor()
    await field('Rule fallback').getByText('Direct if the proxy is unavailable while capture is active.', { exact: true }).waitFor()

    await page.evaluate(() => { window.__test.holdPreview = true })
    await repick.click()
    await dialog.getByRole('status').waitFor()
    assert.equal(await dialog.getByText('Office proxy', { exact: true }).isVisible(), false)
    await repick.click()
    await field('Saved route').getByText('Direct', { exact: true }).waitFor()
    await page.evaluate(() => { window.__test.releasePreview() })
    await dialog.getByText(otherPath, { exact: true }).waitFor()
    await field('Winning match').getByText('No enabled rule matched this file.', { exact: true }).waitFor()
    await field('Winning match').getByText(/Settings stores Block as the default, but capture passes unmatched traffic direct/).waitFor()
    assert.equal(await dialog.getByText('Office proxy', { exact: true }).isVisible(), false)
    assert.deepEqual(await calls(), [
      { method: 'PickExeFile', args: [] },
      ...[browserPath, browserPath, otherPath].flatMap((path) =>
        [{ method: 'PickExeFile', args: [] }, { method: 'PreviewRoute', args: [path] }]),
    ])
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})

test('Windows boot persists independently of sign-in and engine state; blocked listeners are disposed', async () => {
  const { page, errors } = await openGUI({ wizardDone: true })
  try {
    await page.getByRole('link', { name: /Settings$/ }).click()
    await page.getByRole('heading', { name: 'Settings', exact: true }).waitFor()
    const boot = page.getByRole('switch', { name: 'Apply routing rules when Windows starts', exact: true })
    const signIn = page.getByRole('switch', { name: 'Open Proxity at sign-in', exact: true })
    for (const enabled of [false, true]) {
      assert.equal(await boot.isChecked(), !enabled)
      await boot.click()
      await page.waitForFunction((value) => window.__test.settings.auto_start_service === value, enabled)
      await page.getByRole('link', { name: /Dashboard$/ }).click()
      await page.getByRole('heading', { name: 'Dashboard', exact: true }).waitFor()
      await page.getByRole('link', { name: /Settings$/ }).click()
      assert.equal(await boot.isChecked(), enabled)
      assert.equal(await signIn.isChecked(), false)
    }
    const calls = await page.evaluate(() => window.__test.calls)
    assert.deepEqual(calls.filter((c) => c.method === 'UpdateSettings').map(({ args: [s] }) =>
      [s.auto_start_service, s.auto_start_gui]), [[false, false], [true, false]])
    assert.deepEqual(calls.filter((c) => ['InstallService', 'UninstallService', 'StartEngine', 'StopEngine', 'SetGuiAutoStart'].includes(c.method)), [])
    for (let i = 0; i < 3; i++) {
      await page.getByRole('link', { name: /Blocked$/ }).click()
      await page.getByRole('heading', { name: 'Blocked', exact: true, level: 1 }).waitFor()
      assert.equal(await page.evaluate(() => window.__test.events['svc:blocked'].length), 1)
      await page.getByRole('link', { name: /Dashboard$/ }).click()
      await page.getByRole('heading', { name: 'Dashboard', exact: true }).waitFor()
      assert.equal(await page.evaluate(() => window.__test.events['svc:blocked'].length), 0)
    }
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})

test('reconnection refreshes settings and null connection events clear stale entries', async () => {
  const { page, errors } = await openGUI({ wizardDone: true, connected: false })
  try {
    await page.getByRole('link', { name: /Settings$/ }).click()
    await page.getByText('Proxity service is unreachable', { exact: true }).waitFor()
    await page.evaluate(() => {
      window.__test.settings.theme = 'dark'
      window.__test.connected = true
      window.__test.events['svc:connected'].forEach((fn) => fn())
    })
    await page.getByRole('heading', { name: 'Settings', exact: true }).waitFor()
    assert.equal(await page.locator('select').first().inputValue(), 'dark')
    const count = await page.evaluate(async () => {
      const { useApp } = await import('/src/stores/app.ts')
      useApp().conns = [{ ID: 1 }]
      window.__test.events['svc:conns'].forEach((fn) => fn(null))
      return useApp().conns.length
    })
    assert.equal(count, 0)
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})

test('proxy form needs only endpoint and credentials and cannot save while testing', async () => {
  const { page, errors } = await openGUI({ wizardDone: true })
  try {
    await page.getByRole('link', { name: /Proxies$/ }).click()
    await page.getByRole('button', { name: /Add Proxy/ }).first().click()
    const modal = page.getByRole('dialog')
    await modal.locator('input').nth(1).fill('proxy.example.com')
    await modal.locator('input').nth(2).fill('1234')
    await modal.locator('input').nth(3).fill('alice')
    await modal.locator('input').nth(4).fill('secret')
    await page.evaluate(() => { window.__test.testDelay = 500 })
    await modal.getByRole('button', { name: 'Test Connection', exact: true }).click()
    assert.equal(await modal.getByRole('button', { name: 'Save', exact: true }).isDisabled(), true)
    await page.getByText('Connected — 12 ms, exit IP: 203.0.113.1', { exact: true }).waitFor()
    await modal.getByRole('button', { name: 'Save', exact: true }).click()
    await modal.waitFor({ state: 'hidden' })
    const calls = await page.evaluate(() => window.__test.calls.filter((c) => c.method === 'CreateProfile'))
    assert.equal(calls.length, 1)
    assert.equal(calls[0].args[0].name, 'proxy.example.com:1234')
    assert.equal(calls[0].args[0].password, 'secret')
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})

test('cancel a conflicting rule edit restores the original rule', async () => {
  const rule = { ID: 'rule-1', Name: 'Original', Kind: 'exe', Path: 'C:\\Apps\\app.exe',
    Action: 'direct', ProxyID: '', Enabled: true, KillSwitch: false, IncludeSubdirs: true, Priority: 0 }
  const { page, errors } = await openGUI({ wizardDone: true, rules: [rule] })
  try {
    await page.getByRole('link', { name: /Applications$/ }).click()
    await page.getByRole('button', { name: 'Edit', exact: true }).click()
    await page.getByRole('dialog').locator('input').first().fill('Changed')
    await page.getByRole('button', { name: 'Save', exact: true }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Cancel', exact: true }).click()
    await page.waitForFunction(() => window.__test.calls.filter((c) => c.method === 'UpdateRule').length === 2)
    assert.deepEqual(await page.evaluate(() => window.__test.rules[0]), rule)
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})

test('failed preference write keeps setup open and reports the error', async () => {
  const { page, errors } = await openGUI()
  try {
    await page.evaluate(() => { window.__test.failPrefs = true })
    await page.getByRole('button', { name: 'Skip', exact: true }).click()
    await page.getByRole('alertdialog').getByRole('button', { name: 'Skip', exact: true }).click()
    await page.getByText('Cannot save preferences', { exact: true }).waitFor()
    assert.equal(await page.locator('h1').innerText(), 'Proxity Setup')
    assert.equal(await page.evaluate(() => window.__test.prefs.wizard_done), false)
    assert.deepEqual(errors, [])
  } finally { await page.close() }
})
