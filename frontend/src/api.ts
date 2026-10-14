// Thin typed wrapper over window.go.main.App (the Wails-bound Go object) and
// window.runtime events. Errors coming back from IPC are JSON-encoded
// {code,user_msg,detail} objects; parseError extracts them for friendly
// display.

import type {
  BlockedEntry,
  Connection,
  EngineStatus,
  ExeDailyStats,
  ExportProfile,
  GuiPrefs,
  ImportResult,
  LiveSnapshot,
  PathInfo,
  ProcessInfo,
  ProfileInput,
  ProxyDailyStats,
  ProxyProfile,
  ProxyStatus,
  Rule,
  RuleInput,
  RuleResult,
  ServiceStatus,
  Settings,
  TestResult,
} from './types'

declare global {
  interface Window {
    go: any
    runtime: {
      EventsOn(name: string, cb: (data?: any) => void): () => void
      EventsOff(name: string): void
      EventsEmit(name: string, data?: any): void
    }
  }
}

function app(): any {
  return (window as any).go?.main?.App ?? {}
}

export function onEvent(name: string, cb: (data?: any) => void): () => void {
  if (window.runtime?.EventsOn) {
    return window.runtime.EventsOn(name, cb)
  }
  return () => {}
}

export interface ParsedError {
  msg: string
  code: string
  detail: string
}

// parseError unwraps the JSON-encoded ipc.ErrObj the backend throws; unknown
// shapes fall back to their .message / toString.
export function parseError(e: unknown): ParsedError {
  const raw = e instanceof Error ? e.message : String(e ?? '')
  try {
    const o = JSON.parse(raw)
    if (o && typeof o === 'object' && ('user_msg' in o || 'code' in o)) {
      return {
        msg: String(o.user_msg || o.code || raw),
        code: String(o.code || ''),
        detail: String(o.detail || ''),
      }
    }
  } catch {
    /* not JSON */
  }
  if (raw === 'uac.cancelled') {
    return { msg: 'uac.cancelled', code: 'uac.cancelled', detail: '' }
  }
  return { msg: raw, code: '', detail: '' }
}

export const api = {
  // profiles
  getProfiles: () => app().GetProfiles() as Promise<ProxyProfile[]>,
  createProfile: (in_: ProfileInput) => app().CreateProfile(in_) as Promise<ProxyProfile>,
  updateProfile: (id: string, in_: ProfileInput) =>
    app().UpdateProfile(id, in_) as Promise<ProxyProfile>,
  deleteProfile: (id: string) => app().DeleteProfile(id) as Promise<void>,
  setProfileEnabled: (id: string, enabled: boolean) =>
    app().SetProfileEnabled(id, enabled) as Promise<void>,
  testProfile: (id: string) => app().TestProfile(id) as Promise<TestResult>,
  getProfileStatuses: () => app().GetProfileStatuses() as Promise<ProxyStatus[]>,

  // rules
  getRules: () => app().GetRules() as Promise<Rule[]>,
  createRule: (in_: RuleInput) => app().CreateRule(in_) as Promise<RuleResult>,
  updateRule: (id: string, in_: RuleInput) =>
    app().UpdateRule(id, in_) as Promise<RuleResult>,
  deleteRule: (id: string) => app().DeleteRule(id) as Promise<void>,
  setRuleEnabled: (id: string, enabled: boolean) =>
    app().SetRuleEnabled(id, enabled) as Promise<void>,
  setRuleKillSwitch: (id: string, enabled: boolean) =>
    app().SetRuleKillSwitch(id, enabled) as Promise<void>,
  listProcesses: () => app().ListProcesses() as Promise<ProcessInfo[]>,

  // engine / settings
  startEngine: () => app().StartEngine() as Promise<EngineStatus>,
  stopEngine: () => app().StopEngine() as Promise<EngineStatus>,
  engineStatus: () => app().EngineStatus() as Promise<EngineStatus>,
  getSettings: () => app().GetSettings() as Promise<Settings>,
  updateSettings: (st: Settings) => app().UpdateSettings(st) as Promise<Settings>,

  // stats / data
  getSnapshot: () => app().GetSnapshot() as Promise<LiveSnapshot>,
  getHistory: () => app().GetHistory() as Promise<LiveSnapshot[]>,
  getConnections: () => app().GetConnections() as Promise<Connection[]>,
  getExeStats: (from: string, to: string) =>
    app().GetExeStats(from, to) as Promise<ExeDailyStats[]>,
  getProxyStats: (from: string, to: string) =>
    app().GetProxyStats(from, to) as Promise<ProxyDailyStats[]>,
  resetStats: (exePath: string) => app().ResetStats(exePath) as Promise<void>,
  getBlocked: (limit: number) => app().GetBlocked(limit) as Promise<BlockedEntry[]>,
  clearBlocked: () => app().ClearBlocked() as Promise<void>,
  exportProfiles: (includeSecrets: boolean) =>
    app().ExportProfiles(includeSecrets) as Promise<ExportProfile[]>,
  importProfiles: (profiles: ExportProfile[], overwrite: boolean) =>
    app().ImportProfiles(profiles, overwrite) as Promise<ImportResult>,

  // local helpers
  pickExeFile: () => app().PickExeFile() as Promise<string>,
  pickFolder: () => app().PickFolder() as Promise<string>,
  pickJSONFile: () => app().PickJSONFile() as Promise<string>,
  pickSaveFile: (defaultName: string, pattern: string) =>
    app().PickSaveFile(defaultName, pattern) as Promise<string>,
  pathInfo: (path: string) => app().PathInfo(path) as Promise<PathInfo>,
  readTextFile: (path: string) => app().ReadTextFile(path) as Promise<string>,
  writeTextFile: (path: string, contents: string) =>
    app().WriteTextFile(path, contents) as Promise<void>,
  getExeIcon: (path: string) => app().GetExeIcon(path) as Promise<string>,

  // service / app
  serviceAvailable: () => app().ServiceAvailable() as Promise<boolean>,
  serviceStatus: () => app().ServiceStatus() as Promise<ServiceStatus>,
  installService: () => app().InstallService() as Promise<void>,
  uninstallService: () => app().UninstallService() as Promise<void>,
  setGuiAutoStart: (enabled: boolean) => app().SetGuiAutoStart(enabled) as Promise<void>,
  guiAutoStart: () => app().GuiAutoStart() as Promise<boolean>,
  getGuiPrefs: () => app().GetGuiPrefs() as Promise<GuiPrefs>,
  saveGuiPrefs: (p: GuiPrefs) => app().SaveGuiPrefs(p) as Promise<void>,
  connected: () => app().Connected() as Promise<boolean>,
  quit: () => app().Quit() as Promise<void>,
  showWindow: () => app().ShowWindow() as Promise<void>,
}
