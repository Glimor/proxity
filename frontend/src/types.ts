// Mirrors of the Go wire shapes (internal/domain, internal/appsvc).
// Domain structs without json tags marshal PascalCase; inputs/settings/stats
// marshal snake_case.

export type RuleAction = 'proxy' | 'direct' | 'block'
export type RuleKind = 'exe' | 'folder'
export type ProxyStateNum = 0 | 1 | 2 | 3 // disconnected, connecting, connected, error
export type Decision = 'proxy' | 'direct' | 'block' | 'drop'
export type ConnStatus = 'active' | 'closed' | 'blocked' | 'failed'
export type Language = 'en'

export interface ProxyProfile {
  ID: string
  Name: string
  Host: string
  Port: number
  Username: string
  Enabled: boolean
  TimeoutMS: number
  RetryIntervalMS: number
  MaxRetries: number
  CreatedAt: string
  UpdatedAt: string
}

export interface ProxyStatus {
  ProfileID: string
  State: ProxyStateNum
  LatencyMS: number
  PublicIP: string
  LastError: string
  Since: string
}

export interface Rule {
  ID: string
  Name: string
  Kind: RuleKind
  Path: string
  PathFold: string
  Action: RuleAction
  ProxyID: string
  Enabled: boolean
  KillSwitch: boolean
  IncludeSubdirs: boolean
  Priority: number
  CreatedAt: string
  UpdatedAt: string
}

export interface ProfileInput {
  name: string
  host: string
  port: number
  username: string
  password: string | null // null = keep existing (update) / none (create)
  enabled: boolean
  timeout_ms: number
  retry_interval_ms: number
  max_retries: number
}

export interface RuleInput {
  name: string
  kind: RuleKind
  path: string
  action: RuleAction
  proxy_id: string
  enabled: boolean
  kill_switch: boolean
  include_subdirs: boolean
  priority: number
}

export interface RuleResult {
  rule: Rule | null
  warnings?: string[]
  conflicts?: Rule[]
}

export interface Settings {
  language: Language
  theme: string
  kill_switch_global: boolean
  lan_access: boolean
  secure_dns: boolean
  dns_upstream: string
  dns_via_proxy_id: string
  notifications_enabled: boolean
  notify_blocked: boolean
  notify_proxy_events: boolean
  auto_start_service: boolean
  auto_start_gui: boolean
  start_minimized: boolean
  default_action: Decision
  log_level: string
}

export interface EngineStatus {
  running: boolean
  capture_alive: boolean
  listeners: number
}

export interface FlowStats {
  up_bps: number
  down_bps: number
  total_up: number
  total_down: number
  conns: number
}

export interface LiveSnapshot {
  total_up_bps: number
  total_down_bps: number
  active_conns: number
  per_exe: Record<string, FlowStats>
  per_proxy: Record<string, FlowStats>
  blocked_count: number
  failed_count: number
  succeeded_count: number
}

export interface Connection {
  ID: number
  PID: number
  ExePath: string
  DstIP: string
  DstPort: number
  IPv6: boolean
  Action: Decision
  ProxyID: string
  MatchedRuleID: string
  Status: ConnStatus
  StartedAt: string
  DurationMS: number
  BytesUp: number
  BytesDown: number
}

export interface BlockedEntry {
  ID: number
  TS: string
  ExePath: string
  PID: number
  Dst: string
  Port: number
  Proto: string
  Reason: string
}

export interface ExeDailyStats {
  day: string
  exe_path: string
  proxy_id: string
  down: number
  up: number
  conns: number
  seconds: number
}

export interface ProxyDailyStats {
  day: string
  proxy_id: string
  down: number
  up: number
  conns: number
  seconds: number
}

export interface SessionCounters {
  blocked: number
  failed: number
  succeeded: number
}

export interface LastSession {
  ended_at: string
  snapshot: LiveSnapshot
  counters: SessionCounters
}

export interface ProcessInfo {
  pid: number
  name: string
  path: string
}

export interface TestResult {
  latency_ms: number
  public_ip: string
  error?: string
}

export interface ExportProfile {
  id: string
  name: string
  host: string
  port: number
  username: string
  password_ciphertext?: string // base64
  enabled: boolean
  timeout_ms: number
  retry_interval_ms: number
  max_retries: number
}

export interface ImportResult {
  created: number
  updated: number
  skipped?: string[]
}

export interface ServiceStatus {
  binary_found: boolean
  binary_path?: string
  installed: boolean
  running: boolean
  disabled: boolean
}

export interface GuiPrefs {
  language: Language
  theme: string
  wizard_done: boolean
  start_minimized: boolean
}

export interface PathInfo {
  exists: boolean
  is_dir: boolean
  ext: string
  name: string
}
