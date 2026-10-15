package domain

import "time"

type ProxyProtocol string

const (
	ProxyHTTP   ProxyProtocol = "http"
	ProxyHTTPS  ProxyProtocol = "https"
	ProxySOCKS4 ProxyProtocol = "socks4"
	ProxySOCKS5 ProxyProtocol = "socks5"
)

// Normalized preserves compatibility with profiles created before protocol selection.
func (p ProxyProtocol) Normalized() ProxyProtocol {
	if p == "" {
		return ProxySOCKS5
	}
	return p
}

func (p ProxyProtocol) Valid() bool {
	switch p.Normalized() {
	case ProxyHTTP, ProxyHTTPS, ProxySOCKS4, ProxySOCKS5:
		return true
	}
	return false
}

type ProxyProfile struct {
	ID                 string
	Name               string
	Protocol           ProxyProtocol
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
