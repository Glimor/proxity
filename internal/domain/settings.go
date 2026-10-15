package domain

type Language string

const (
	LangEN Language = "en"
)

type Settings struct {
	Language         Language `json:"language"`
	Theme            string   `json:"theme"` // "dark" | "light" | "system"
	KillSwitchGlobal bool     `json:"kill_switch_global"`
	LANAccess        bool     `json:"lan_access"`
	SecureDNS        bool     `json:"secure_dns"`
	DNSUpstream      string   `json:"dns_upstream"`
	DNSViaProxyID    string   `json:"dns_via_proxy_id"`

	NotificationsEnabled bool `json:"notifications_enabled"`
	NotifyBlocked        bool `json:"notify_blocked"`
	NotifyProxyEvents    bool `json:"notify_proxy_events"`

	AutoStartService bool `json:"auto_start_service"`
	AutoStartGUI     bool `json:"auto_start_gui"`
	StartMinimized   bool `json:"start_minimized"`

	DefaultAction Decision `json:"default_action"` // DecisionDirect | DecisionBlock
	LogLevel      string   `json:"log_level"`      // "debug" | "info" | "warn" | "error"
}

func DefaultSettings() Settings {
	return Settings{
		Language:             LangEN,
		Theme:                "system",
		KillSwitchGlobal:     false,
		LANAccess:            true,
		SecureDNS:            false,
		NotificationsEnabled: true,
		NotifyBlocked:        true,
		NotifyProxyEvents:    true,
		DefaultAction:        DecisionDirect,
		LogLevel:             "info",
	}
}

func (s *Settings) Normalize() {
	if s.Language != LangEN {
		s.Language = LangEN
	}
}
