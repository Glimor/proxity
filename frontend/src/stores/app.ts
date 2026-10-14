import { defineStore } from 'pinia'
import { api, onEvent, parseError } from '../api'
import type {
  Connection,
  EngineStatus,
  GuiPrefs,
  LiveSnapshot,
  ProxyStatus,
  Settings,
} from '../types'

export interface Toast {
  id: number
  kind: 'ok' | 'err' | 'info'
  msg: string
  detail: string
}

export interface ConfirmReq {
  title: string
  body: string
  okLabel: string
  danger: boolean
  resolve: (ok: boolean) => void
}

let toastID = 0

function applyTheme(theme: string) {
  const dark =
    theme === 'dark' ||
    (theme !== 'light' && window.matchMedia('(prefers-color-scheme: dark)').matches)
  document.documentElement.classList.toggle('dark', dark)
}

export const useApp = defineStore('app', {
  state: () => ({
    ready: false,
    connected: false,
    prefs: { language: 'en', theme: 'system', wizard_done: false, start_minimized: false } as GuiPrefs,
    settings: null as Settings | null,
    engine: { running: false, capture_alive: false, listeners: 0 } as EngineStatus,
    snapshot: null as LiveSnapshot | null,
    history: [] as LiveSnapshot[],
    proxyStatuses: {} as Record<string, ProxyStatus>,
    conns: [] as Connection[],
    protectedApps: 0,
    pendingDrops: [] as string[],
    toasts: [] as Toast[],
    confirm: null as ConfirmReq | null,
    themeMedia: null as MediaQueryList | null,
  }),

  getters: {
    killSwitchOn: (s) => s.settings?.kill_switch_global ?? false,
  },

  actions: {
    async init() {
      try {
        this.prefs = await api.getGuiPrefs()
      } catch {
        /* defaults */
      }
      applyTheme(this.prefs.theme)
      this.themeMedia = window.matchMedia('(prefers-color-scheme: dark)')
      this.themeMedia.addEventListener('change', () =>
        applyTheme(this.settings?.theme ?? this.prefs.theme),
      )
      try {
        this.connected = await api.connected()
      } catch {
        this.connected = false
      }
      this.bindEvents()
      if (this.connected) await this.refreshAll()
      this.ready = true
    },

    bindEvents() {
      onEvent('svc:connected', async () => {
        this.connected = true
        await this.refreshAll()
      })
      onEvent('svc:disconnected', () => {
        this.connected = false
      })
      onEvent('svc:stats', (snap: LiveSnapshot) => {
        if (!snap) return
        this.snapshot = snap
        this.history.push(snap)
        if (this.history.length > 90) this.history.splice(0, this.history.length - 90)
      })
      onEvent('svc:status', async (st: EngineStatus) => {
        if (st) this.engine = st
        await this.refreshProxyStatuses()
      })
      onEvent('svc:conns', (list: Connection[]) => {
        if (Array.isArray(list)) this.conns = list
      })
      onEvent('gui:filedrop', (paths: string[]) => {
        if (Array.isArray(paths) && paths.length) this.pendingDrops = paths
      })
    },

    async refreshAll() {
      await Promise.allSettled([
        this.refreshSettings(),
        this.refreshEngine(),
        this.refreshProxyStatuses(),
        this.refreshRulesCount(),
        api.getSnapshot().then((s) => (this.snapshot = s)),
        api.getHistory().then((h) => (this.history = (h ?? []).slice(-90))),
        api.getConnections().then((c) => (this.conns = c ?? [])),
      ])
    },

    async refreshSettings() {
      this.settings = await api.getSettings()
      // Service wins for shared keys; keep gui.json as the boot fallback.
      applyTheme(this.settings.theme || this.prefs.theme)
      this.prefs = {
        ...this.prefs,
        language: 'en',
        theme: this.settings.theme || this.prefs.theme,
        start_minimized: this.settings.start_minimized,
      }
    },

    async refreshEngine() {
      try {
        this.engine = await api.engineStatus()
      } catch {
        /* offline */
      }
    },

    async refreshProxyStatuses() {
      try {
        const list = await api.getProfileStatuses()
        const map: Record<string, ProxyStatus> = {}
        for (const s of list ?? []) map[s.ProfileID] = s
        this.proxyStatuses = map
      } catch {
        /* offline */
      }
    },

    async refreshRulesCount() {
      try {
        const rules = await api.getRules()
        this.protectedApps = (rules ?? []).filter((r) => r.Enabled && r.Action === 'proxy').length
      } catch {
        /* offline */
      }
    },

    toast(kind: Toast['kind'], msg: string, detail = '') {
      const t: Toast = { id: ++toastID, kind, msg, detail }
      this.toasts.push(t)
      setTimeout(() => {
        this.toasts = this.toasts.filter((x) => x.id !== t.id)
      }, detail ? 9000 : 4500)
    },

    toastError(e: unknown, fallback: string) {
      const p = parseError(e)
      this.toast('err', p.msg || fallback, p.detail)
    },

    askConfirm(title: string, body: string, okLabel: string, danger = true): Promise<boolean> {
      return new Promise((resolve) => {
        this.confirm = { title, body, okLabel, danger, resolve }
      })
    },

    answerConfirm(ok: boolean) {
      this.confirm?.resolve(ok)
      this.confirm = null
    },

    async savePrefs(patch: Partial<GuiPrefs>) {
      this.prefs = { ...this.prefs, ...patch }
      try {
        await api.saveGuiPrefs(this.prefs)
      } catch {
        /* best effort */
      }
    },

    async setTheme(theme: string) {
      applyTheme(theme)
      await this.savePrefs({ theme })
      if (this.settings) {
        this.settings.theme = theme
        try {
          await api.updateSettings(this.settings)
        } catch {
          /* offline */
        }
      }
    },
  },
})
