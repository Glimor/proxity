package domain

import "time"

type Decision string

const (
	DecisionProxy  Decision = "proxy"
	DecisionDirect Decision = "direct"
	DecisionBlock  Decision = "block"
	DecisionDrop   Decision = "drop"
)

type ConnStatus string

const (
	ConnActive  ConnStatus = "active"
	ConnClosed  ConnStatus = "closed"
	ConnBlocked ConnStatus = "blocked"
	ConnFailed  ConnStatus = "failed"
)

type Connection struct {
	ID            int64
	PID           int
	ExePath       string
	DstIP         string
	DstPort       int
	IPv6          bool
	Action        Decision
	ProxyID       string
	MatchedRuleID string
	Status        ConnStatus
	StartedAt     time.Time
	DurationMS    int64
	BytesUp       int64
	BytesDown     int64
}

type BlockedEntry struct {
	ID      int64
	TS      time.Time
	ExePath string
	PID     int
	Dst     string
	Port    int
	Proto   string
	Reason  string
}
