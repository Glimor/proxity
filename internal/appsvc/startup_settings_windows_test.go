package appsvc

import (
	"context"
	"errors"
	"testing"

	"github.com/Glimor/proxity/internal/domain"
)

type fakeStartup struct {
	automatic bool
	changeErr error
}

func (f *fakeStartup) Automatic() (bool, error) { return f.automatic, nil }

func (f *fakeStartup) SetAutomatic(enabled bool) (func() error, error) {
	if f.changeErr != nil {
		return nil, f.changeErr
	}
	previous := f.automatic
	f.automatic = enabled
	return func() error {
		f.automatic = previous
		return nil
	}, nil
}

type settingsWithFailedSave struct {
	SettingsAPI
	err error
}

func (s settingsWithFailedSave) SaveSettings(context.Context, domain.Settings) error {
	return s.err
}

func TestFailedStartupChangeDoesNotPersistSettings(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := t.Context()
	rejected := errors.New("SCM rejected change")
	initial, err := svc.d.Settings.LoadSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	svc.d.Startup = &fakeStartup{automatic: initial.AutoStartService, changeErr: rejected}
	request := initial
	request.AutoStartService = !initial.AutoStartService
	request.Theme = "dark"

	if _, err := svc.UpdateSettings(ctx, request); !errors.Is(err, rejected) {
		t.Fatalf("update error = %v, want SCM failure", err)
	}
	got, err := svc.d.Settings.LoadSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got != initial {
		t.Fatalf("rejected update persisted settings: got %+v, want %+v", got, initial)
	}
}

func TestStartupUsesSCMDespiteLegacySettings(t *testing.T) {
	for _, tc := range []struct {
		name      string
		automatic bool
	}{
		{name: "legacy false with automatic", automatic: true},
		{name: "legacy true with manual", automatic: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newTestService(t)
			ctx := t.Context()
			startup := &fakeStartup{
				automatic: tc.automatic,
				changeErr: errors.New("QUERY_CONFIG granted but CHANGE_CONFIG denied"),
			}
			svc.d.Startup = startup
			stored := domain.DefaultSettings()
			stored.AutoStartService = !tc.automatic
			if err := svc.d.Settings.SaveSettings(ctx, stored); err != nil {
				t.Fatal(err)
			}
			got, err := svc.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if got.AutoStartService != tc.automatic {
				t.Fatalf("startup = %v, want actual SCM mode %v", got.AutoStartService, tc.automatic)
			}
			got.Theme = "dark"
			if _, err := svc.UpdateSettings(ctx, got); err != nil {
				t.Fatal(err)
			}
			if startup.automatic != tc.automatic {
				t.Fatal("unrelated edit changed the installed startup policy")
			}
		})
	}
}

func TestStartupToggleChangesOnlyNextBoot(t *testing.T) {
	for name, enable := range map[string]bool{"enable": true, "disable": false} {
		t.Run(name, func(t *testing.T) {
			svc, eng, _ := newTestService(t)
			ctx := t.Context()
			startup := &fakeStartup{automatic: !enable}
			svc.d.Startup = startup
			if err := svc.StartEngine(ctx); err != nil {
				t.Fatal(err)
			}
			st := domain.DefaultSettings()
			st.AutoStartService = enable

			got, err := svc.UpdateSettings(ctx, st)
			if err != nil {
				t.Fatal(err)
			}
			persisted, err := svc.d.Settings.LoadSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if startup.automatic != enable || got.AutoStartService != enable || persisted.AutoStartService != enable {
				t.Fatalf("wanted automatic = %v; SCM = %v, returned = %v, persisted = %v",
					enable, startup.automatic, got.AutoStartService, persisted.AutoStartService)
			}
			if !eng.Running() {
				t.Fatal("startup toggle stopped current routing")
			}
		})
	}
}

func TestFailedSettingsSaveRestoresStartup(t *testing.T) {
	svc, _, _ := newTestService(t)
	saveErr := errors.New("settings store unavailable")
	svc.d.Settings = settingsWithFailedSave{SettingsAPI: svc.d.Settings, err: saveErr}
	st := domain.DefaultSettings()
	st.AutoStartService = false

	if _, err := svc.UpdateSettings(t.Context(), st); !errors.Is(err, saveErr) {
		t.Fatalf("update error = %v, want settings store failure", err)
	}
	automatic, err := svc.d.Startup.Automatic()
	if err != nil {
		t.Fatal(err)
	}
	if !automatic {
		t.Fatal("failed settings save left SCM in manual mode")
	}
}
