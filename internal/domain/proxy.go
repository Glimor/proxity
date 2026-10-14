package domain

import "time"

type ProxyProfile struct {
	ID                 string
	Name               string
	Host               string
	Port               int
	Username           string
	PasswordCiphertext []byte
	Enabled            bool
	TimeoutMS          int
	RetryIntervalMS    int
	MaxRetries         int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type ProxyState int

const (
	ProxyDisconnected ProxyState = iota
	ProxyConnecting
	ProxyConnected
	ProxyError
)

func (s ProxyState) String() string {
	switch s {
	case ProxyConnecting:
		return "connecting"
	case ProxyConnected:
		return "connected"
	case ProxyError:
		return "error"
	default:
		return "disconnected"
	}
}

func ParseProxyState(v string) ProxyState {
	switch v {
	case "connecting":
		return ProxyConnecting
	case "connected":
		return ProxyConnected
	case "error":
		return ProxyError
	default:
		return ProxyDisconnected
	}
}

type ProxyStatus struct {
	ProfileID string
	State     ProxyState
	LatencyMS int64
	PublicIP  string
	LastError string
	Since     time.Time
}
