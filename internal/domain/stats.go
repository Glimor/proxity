package domain

import "time"

type FlowStats struct {
	UpBps     int64 `json:"up_bps"`
	DownBps   int64 `json:"down_bps"`
	TotalUp   int64 `json:"total_up"`
	TotalDown int64 `json:"total_down"`
	Conns     int   `json:"conns"`
}

type LiveSnapshot struct {
	TotalUpBps     int64                `json:"total_up_bps"`
	TotalDownBps   int64                `json:"total_down_bps"`
	ActiveConns    int                  `json:"active_conns"`
	PerExe         map[string]FlowStats `json:"per_exe"`
	PerProxy       map[string]FlowStats `json:"per_proxy"`
	BlockedCount   int64                `json:"blocked_count"`
	FailedCount    int64                `json:"failed_count"`
	SucceededCount int64                `json:"succeeded_count"`
}

type ExeDailyStats struct {
	Day     string `json:"day"` // YYYY-MM-DD, local time
	ExePath string `json:"exe_path"`
	ProxyID string `json:"proxy_id"`
	Down    int64  `json:"down"`
	Up      int64  `json:"up"`
	Conns   int64  `json:"conns"`
	Seconds int64  `json:"seconds"`
}

type ProxyDailyStats struct {
	Day     string `json:"day"`
	ProxyID string `json:"proxy_id"`
	Down    int64  `json:"down"`
	Up      int64  `json:"up"`
	Conns   int64  `json:"conns"`
	Seconds int64  `json:"seconds"`
}

type SessionCounters struct {
	Blocked   int64 `json:"blocked"`
	Failed    int64 `json:"failed"`
	Succeeded int64 `json:"succeeded"`
}

// LastSession is the persisted snapshot of the previous engine run, stored
// in session_stats under "last_session" at service shutdown.
type LastSession struct {
	EndedAt  time.Time       `json:"ended_at"`
	Snapshot LiveSnapshot    `json:"snapshot"`
	Counters SessionCounters `json:"counters"`
}
