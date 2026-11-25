package appsvc

import (
	"context"
	"errors"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/svccfg"
)

// StartupAPI controls the service's next-boot policy, not its running state.
type StartupAPI interface {
	Automatic() (bool, error)
	SetAutomatic(enabled bool) (rollback func() error, err error)
}

func (s *Service) GetSettings(ctx context.Context) (domain.Settings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	return s.loadSettings(ctx)
}

func (s *Service) loadSettings(ctx context.Context) (domain.Settings, error) {
	st, err := s.d.Settings.LoadSettings(ctx)
	if err != nil {
		return st, err
	}
	automatic, err := s.d.Startup.Automatic()
	switch {
	case err == nil:
		// Legacy settings can disagree with what Windows actually starts.
		st.AutoStartService = automatic
	case errors.Is(err, svccfg.ErrNotInstalled):
		// There is no next-boot behavior to enable until recovery installs it.
		st.AutoStartService = false
	default:
		return st, domain.WrapError("settings.autostart_failed",
			"The service start mode could not be read.", err)
	}
	return st, nil
}

func (s *Service) UpdateSettings(ctx context.Context, st domain.Settings) (domain.Settings, error) {
	s.settingsMu.Lock()
	defer s.settingsMu.Unlock()
	st.Normalize()
	prev, err := s.loadSettings(ctx)
	if err != nil {
		return st, err
	}
	var rollback func() error
	if st.AutoStartService != prev.AutoStartService {
		rollback, err = s.d.Startup.SetAutomatic(st.AutoStartService)
		if err != nil {
			return st, domain.WrapError("settings.autostart_failed",
				"The service start mode could not be changed.", err)
		}
	}
	if err := s.d.Settings.SaveSettings(ctx, st); err != nil {
		if rollback != nil {
			if restoreErr := rollback(); restoreErr != nil {
				return st, domain.WrapError("settings.autostart_failed",
					"Settings could not be saved and the previous service start mode could not be restored.",
					errors.Join(err, restoreErr))
			}
		}
		return st, err
	}
	s.refreshConfig(ctx)
	return st, nil
}
