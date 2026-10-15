package sqlite

import "github.com/Glimor/proxity/internal/ports"

var (
	_ ports.ProfileStore    = (*ProfileRepo)(nil)
	_ ports.RuleStore       = (*RuleRepo)(nil)
	_ ports.SettingsStore   = (*SettingsRepo)(nil)
	_ ports.StatsStore      = (*StatsRepo)(nil)
	_ ports.BlockedLogStore = (*BlockedRepo)(nil)
)
