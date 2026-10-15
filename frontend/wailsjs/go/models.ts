export namespace appsvc {
	
	export class EngineStatus {
	    running: boolean;
	    capture_alive: boolean;
	    listeners: number;
	
	    static createFrom(source: any = {}) {
	        return new EngineStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.capture_alive = source["capture_alive"];
	        this.listeners = source["listeners"];
	    }
	}
	export class ExportProfile {
	    protocol: string;
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    password_ciphertext?: number[];
	    enabled: boolean;
	    timeout_ms: number;
	    retry_interval_ms: number;
	    max_retries: number;
	
	    static createFrom(source: any = {}) {
	        return new ExportProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol = source["protocol"];
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password_ciphertext = source["password_ciphertext"];
	        this.enabled = source["enabled"];
	        this.timeout_ms = source["timeout_ms"];
	        this.retry_interval_ms = source["retry_interval_ms"];
	        this.max_retries = source["max_retries"];
	    }
	}
	export class ImportResult {
	    created: number;
	    updated: number;
	    skipped?: string[];
	
	    static createFrom(source: any = {}) {
	        return new ImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.created = source["created"];
	        this.updated = source["updated"];
	        this.skipped = source["skipped"];
	    }
	}
	export class ProcessInfo {
	    pid: number;
	    name: string;
	    path: string;
	
	    static createFrom(source: any = {}) {
	        return new ProcessInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pid = source["pid"];
	        this.name = source["name"];
	        this.path = source["path"];
	    }
	}
	export class ProfileInput {
	    protocol: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    password?: string;
	    enabled: boolean;
	    timeout_ms: number;
	    retry_interval_ms: number;
	    max_retries: number;
	
	    static createFrom(source: any = {}) {
	        return new ProfileInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol = source["protocol"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.password = source["password"];
	        this.enabled = source["enabled"];
	        this.timeout_ms = source["timeout_ms"];
	        this.retry_interval_ms = source["retry_interval_ms"];
	        this.max_retries = source["max_retries"];
	    }
	}
	export class RuleInput {
	    name: string;
	    kind: string;
	    path: string;
	    action: string;
	    proxy_id: string;
	    enabled: boolean;
	    kill_switch: boolean;
	    include_subdirs: boolean;
	    priority: number;
	
	    static createFrom(source: any = {}) {
	        return new RuleInput(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.kind = source["kind"];
	        this.path = source["path"];
	        this.action = source["action"];
	        this.proxy_id = source["proxy_id"];
	        this.enabled = source["enabled"];
	        this.kill_switch = source["kill_switch"];
	        this.include_subdirs = source["include_subdirs"];
	        this.priority = source["priority"];
	    }
	}
	export class RuleResult {
	    rule?: domain.Rule;
	    warnings?: string[];
	    conflicts?: domain.Rule[];
	
	    static createFrom(source: any = {}) {
	        return new RuleResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule = this.convertValues(source["rule"], domain.Rule);
	        this.warnings = source["warnings"];
	        this.conflicts = this.convertValues(source["conflicts"], domain.Rule);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TestResult {
	    latency_ms: number;
	    public_ip: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new TestResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.latency_ms = source["latency_ms"];
	        this.public_ip = source["public_ip"];
	        this.error = source["error"];
	    }
	}

}

export namespace domain {
	
	export class BlockedEntry {
	    ID: number;
	    // Go type: time
	    TS: any;
	    ExePath: string;
	    PID: number;
	    Dst: string;
	    Port: number;
	    Proto: string;
	    Reason: string;
	
	    static createFrom(source: any = {}) {
	        return new BlockedEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.TS = this.convertValues(source["TS"], null);
	        this.ExePath = source["ExePath"];
	        this.PID = source["PID"];
	        this.Dst = source["Dst"];
	        this.Port = source["Port"];
	        this.Proto = source["Proto"];
	        this.Reason = source["Reason"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Connection {
	    ID: number;
	    PID: number;
	    ExePath: string;
	    DstIP: string;
	    DstPort: number;
	    IPv6: boolean;
	    Action: string;
	    ProxyID: string;
	    MatchedRuleID: string;
	    Status: string;
	    // Go type: time
	    StartedAt: any;
	    DurationMS: number;
	    BytesUp: number;
	    BytesDown: number;
	
	    static createFrom(source: any = {}) {
	        return new Connection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.PID = source["PID"];
	        this.ExePath = source["ExePath"];
	        this.DstIP = source["DstIP"];
	        this.DstPort = source["DstPort"];
	        this.IPv6 = source["IPv6"];
	        this.Action = source["Action"];
	        this.ProxyID = source["ProxyID"];
	        this.MatchedRuleID = source["MatchedRuleID"];
	        this.Status = source["Status"];
	        this.StartedAt = this.convertValues(source["StartedAt"], null);
	        this.DurationMS = source["DurationMS"];
	        this.BytesUp = source["BytesUp"];
	        this.BytesDown = source["BytesDown"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ExeDailyStats {
	    day: string;
	    exe_path: string;
	    proxy_id: string;
	    down: number;
	    up: number;
	    conns: number;
	    seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new ExeDailyStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.exe_path = source["exe_path"];
	        this.proxy_id = source["proxy_id"];
	        this.down = source["down"];
	        this.up = source["up"];
	        this.conns = source["conns"];
	        this.seconds = source["seconds"];
	    }
	}
	export class FlowStats {
	    up_bps: number;
	    down_bps: number;
	    total_up: number;
	    total_down: number;
	    conns: number;
	
	    static createFrom(source: any = {}) {
	        return new FlowStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.up_bps = source["up_bps"];
	        this.down_bps = source["down_bps"];
	        this.total_up = source["total_up"];
	        this.total_down = source["total_down"];
	        this.conns = source["conns"];
	    }
	}
	export class SessionCounters {
	    blocked: number;
	    failed: number;
	    succeeded: number;
	
	    static createFrom(source: any = {}) {
	        return new SessionCounters(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.blocked = source["blocked"];
	        this.failed = source["failed"];
	        this.succeeded = source["succeeded"];
	    }
	}
	export class LiveSnapshot {
	    total_up_bps: number;
	    total_down_bps: number;
	    active_conns: number;
	    per_exe: Record<string, FlowStats>;
	    per_proxy: Record<string, FlowStats>;
	    blocked_count: number;
	    failed_count: number;
	    succeeded_count: number;
	
	    static createFrom(source: any = {}) {
	        return new LiveSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_up_bps = source["total_up_bps"];
	        this.total_down_bps = source["total_down_bps"];
	        this.active_conns = source["active_conns"];
	        this.per_exe = this.convertValues(source["per_exe"], FlowStats, true);
	        this.per_proxy = this.convertValues(source["per_proxy"], FlowStats, true);
	        this.blocked_count = source["blocked_count"];
	        this.failed_count = source["failed_count"];
	        this.succeeded_count = source["succeeded_count"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LastSession {
	    // Go type: time
	    ended_at: any;
	    snapshot: LiveSnapshot;
	    counters: SessionCounters;
	
	    static createFrom(source: any = {}) {
	        return new LastSession(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ended_at = this.convertValues(source["ended_at"], null);
	        this.snapshot = this.convertValues(source["snapshot"], LiveSnapshot);
	        this.counters = this.convertValues(source["counters"], SessionCounters);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ProxyDailyStats {
	    day: string;
	    proxy_id: string;
	    down: number;
	    up: number;
	    conns: number;
	    seconds: number;
	
	    static createFrom(source: any = {}) {
	        return new ProxyDailyStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.day = source["day"];
	        this.proxy_id = source["proxy_id"];
	        this.down = source["down"];
	        this.up = source["up"];
	        this.conns = source["conns"];
	        this.seconds = source["seconds"];
	    }
	}
	export class ProxyProfile {
	    Protocol: string;
	    ID: string;
	    Name: string;
	    Host: string;
	    Port: number;
	    Username: string;
	    PasswordCiphertext: number[];
	    Enabled: boolean;
	    TimeoutMS: number;
	    RetryIntervalMS: number;
	    MaxRetries: number;
	    // Go type: time
	    CreatedAt: any;
	    // Go type: time
	    UpdatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new ProxyProfile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Protocol = source["Protocol"];
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Host = source["Host"];
	        this.Port = source["Port"];
	        this.Username = source["Username"];
	        this.PasswordCiphertext = source["PasswordCiphertext"];
	        this.Enabled = source["Enabled"];
	        this.TimeoutMS = source["TimeoutMS"];
	        this.RetryIntervalMS = source["RetryIntervalMS"];
	        this.MaxRetries = source["MaxRetries"];
	        this.CreatedAt = this.convertValues(source["CreatedAt"], null);
	        this.UpdatedAt = this.convertValues(source["UpdatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProxyStatus {
	    ProfileID: string;
	    State: number;
	    LatencyMS: number;
	    PublicIP: string;
	    LastError: string;
	    // Go type: time
	    Since: any;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ProfileID = source["ProfileID"];
	        this.State = source["State"];
	        this.LatencyMS = source["LatencyMS"];
	        this.PublicIP = source["PublicIP"];
	        this.LastError = source["LastError"];
	        this.Since = this.convertValues(source["Since"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Rule {
	    ID: string;
	    Name: string;
	    Kind: string;
	    Path: string;
	    PathFold: string;
	    Action: string;
	    ProxyID: string;
	    Enabled: boolean;
	    KillSwitch: boolean;
	    IncludeSubdirs: boolean;
	    Priority: number;
	    // Go type: time
	    CreatedAt: any;
	    // Go type: time
	    UpdatedAt: any;
	
	    static createFrom(source: any = {}) {
	        return new Rule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ID = source["ID"];
	        this.Name = source["Name"];
	        this.Kind = source["Kind"];
	        this.Path = source["Path"];
	        this.PathFold = source["PathFold"];
	        this.Action = source["Action"];
	        this.ProxyID = source["ProxyID"];
	        this.Enabled = source["Enabled"];
	        this.KillSwitch = source["KillSwitch"];
	        this.IncludeSubdirs = source["IncludeSubdirs"];
	        this.Priority = source["Priority"];
	        this.CreatedAt = this.convertValues(source["CreatedAt"], null);
	        this.UpdatedAt = this.convertValues(source["UpdatedAt"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Settings {
	    language: string;
	    theme: string;
	    kill_switch_global: boolean;
	    lan_access: boolean;
	    secure_dns: boolean;
	    dns_upstream: string;
	    dns_via_proxy_id: string;
	    notifications_enabled: boolean;
	    notify_blocked: boolean;
	    notify_proxy_events: boolean;
	    auto_start_service: boolean;
	    auto_start_gui: boolean;
	    start_minimized: boolean;
	    default_action: string;
	    log_level: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.theme = source["theme"];
	        this.kill_switch_global = source["kill_switch_global"];
	        this.lan_access = source["lan_access"];
	        this.secure_dns = source["secure_dns"];
	        this.dns_upstream = source["dns_upstream"];
	        this.dns_via_proxy_id = source["dns_via_proxy_id"];
	        this.notifications_enabled = source["notifications_enabled"];
	        this.notify_blocked = source["notify_blocked"];
	        this.notify_proxy_events = source["notify_proxy_events"];
	        this.auto_start_service = source["auto_start_service"];
	        this.auto_start_gui = source["auto_start_gui"];
	        this.start_minimized = source["start_minimized"];
	        this.default_action = source["default_action"];
	        this.log_level = source["log_level"];
	    }
	}

}

export namespace guiprefs {
	
	export class Prefs {
	    language: string;
	    theme: string;
	    wizard_done: boolean;
	    start_minimized: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Prefs(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.language = source["language"];
	        this.theme = source["theme"];
	        this.wizard_done = source["wizard_done"];
	        this.start_minimized = source["start_minimized"];
	    }
	}

}

export namespace main {
	
	export class PathInfo {
	    exists: boolean;
	    is_dir: boolean;
	    ext: string;
	    name: string;
	
	    static createFrom(source: any = {}) {
	        return new PathInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exists = source["exists"];
	        this.is_dir = source["is_dir"];
	        this.ext = source["ext"];
	        this.name = source["name"];
	    }
	}
	export class ServiceStatus {
	    binary_found: boolean;
	    binary_path?: string;
	    installed: boolean;
	    running: boolean;
	    disabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ServiceStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.binary_found = source["binary_found"];
	        this.binary_path = source["binary_path"];
	        this.installed = source["installed"];
	        this.running = source["running"];
	        this.disabled = source["disabled"];
	    }
	}

}

