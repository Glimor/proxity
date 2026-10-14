//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Glimor/proxity/internal/appsvc"
	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/guiicon"
	"github.com/Glimor/proxity/internal/guiprefs"
	"github.com/Glimor/proxity/internal/ipc"
	"github.com/Glimor/proxity/internal/version"
	wruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const defaultCallTimeout = 35 * time.Second

// rpcError serializes an IPC error object as JSON so the frontend can show
// user_msg with the technical detail behind an expander.
type rpcError struct{ e *ipc.ErrObj }

func (r *rpcError) Error() string {
	b, err := json.Marshal(r.e)
	if err != nil {
		return r.e.Error()
	}
	return string(b)
}

func asRPCError(err error) error {
	var eo *ipc.ErrObj
	if errors.As(err, &eo) {
		return &rpcError{e: eo}
	}
	if errors.Is(err, ipc.ErrDisconnected) {
		return &rpcError{e: &ipc.ErrObj{
			Code:    "svc.offline",
			UserMsg: "Proxity service is not reachable",
		}}
	}
	return err
}

// App is the Wails bound object: every method is a thin typed wrapper over
// ipc.Call against the service. It also owns GUI-local concerns (prefs,
// tray, notifications, icon cache).
type App struct {
	ctx   context.Context
	cli   *ipc.Client
	prefs guiprefs.Prefs

	connected  atomic.Bool
	quitting   atomic.Bool
	svcVersion atomic.Value // string — service build version from ping

	trayEnd func()
	notify  notifier

	settingsMu sync.RWMutex
	settings   domain.Settings

	iconsMu sync.Mutex
	icons   map[string]string

	trayMu    sync.Mutex
	trayItems *trayItems
}

// NewApp builds the App around a reconnecting IPC client.
func NewApp(p guiprefs.Prefs) *App {
	return &App{
		cli:      ipc.NewClient(""),
		prefs:    p,
		settings: domain.DefaultSettings(),
		icons:    make(map[string]string),
		notify:   newNotifier(),
	}
}

// ---------------------------------------------------------------------------
// lifecycle
// ---------------------------------------------------------------------------

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	a.cli.OnEvent(func(ev ipc.Event) {
		wruntime.EventsEmit(ctx, "svc:"+ev.Event, ev.Data)
		switch ev.Event {
		case "status":
			var st appsvc.EngineStatus
			if b, err := json.Marshal(ev.Data); err == nil && json.Unmarshal(b, &st) == nil {
				a.updateTrayEngine(st.Running)
			}
		case "blocked":
			a.notifyBlocked()
		}
	})
	go a.cli.Run(ctx)
	go a.connWatch(ctx)
	go a.proxyWatch(ctx)

	wruntime.OnFileDrop(ctx, func(_ int, _ int, paths []string) {
		wruntime.EventsEmit(ctx, "gui:filedrop", paths)
	})

	a.startTray()
}

func (a *App) beforeClose(_ context.Context) bool {
	if a.quitting.Load() {
		return false
	}
	if a.ctx != nil {
		wruntime.WindowHide(a.ctx)
	}
	return true // hide to tray instead of quitting
}

func (a *App) shutdown(_ context.Context) {
	a.cli.Close()
	if a.trayEnd != nil {
		a.trayEnd()
	}
}

// connWatch pings the service and emits svc:connected / svc:disconnected on
// transitions; on connect it re-subscribes to the event stream (subscription
// is per-connection and dies on reconnect).
func (a *App) connWatch(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	var once bool
	for {
		if once {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		} else {
			once = true
		}
		ok := a.ping()
		prev := a.connected.Swap(ok)
		switch {
		case ok && !prev:
			_, _ = a.cli.Call("events.subscribe", nil, 5*time.Second)
			a.refreshSettingsCache()
			a.emit("svc:connected", nil)
		case !ok && prev:
			a.svcVersion.Store("")
			a.emit("svc:disconnected", nil)
		}
	}
}

func (a *App) ping() bool {
	raw, err := a.cli.Call("ping", nil, 1500*time.Millisecond)
	if err != nil {
		return false
	}
	var res struct {
		Version string `json:"version"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &res) == nil {
		a.svcVersion.Store(res.Version)
	}
	return true
}

// proxyWatch polls profile statuses and toasts on connect/disconnect edges
// when the user enabled proxy notifications.
func (a *App) proxyWatch(ctx context.Context) {
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	last := make(map[string]domain.ProxyState)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if !a.connected.Load() || !a.notify.proxyEvents() {
			continue
		}
		var statuses []domain.ProxyStatus
		if err := a.call("profiles.statuses", nil, &statuses); err != nil {
			continue
		}
		cur := make(map[string]domain.ProxyState, len(statuses))
		for _, st := range statuses {
			cur[st.ProfileID] = st.State
			prev, seen := last[st.ProfileID]
			if !seen {
				continue
			}
			switch {
			case prev != domain.ProxyConnected && st.State == domain.ProxyConnected:
				a.notify.toast(a.tr("Proxy bağlandı", "Proxy connected"),
					a.tr("Proxy bağlantısı yeniden kuruldu.", "The proxy connection was restored."))
			case prev == domain.ProxyConnected &&
				(st.State == domain.ProxyDisconnected || st.State == domain.ProxyError):
				a.notify.toast(a.tr("Proxy bağlantısı koptu", "Proxy disconnected"),
					a.tr("Proxy ile bağlantı kesildi; Proxity yeniden bağlanmayı deniyor.",
						"The proxy connection was lost; Proxity is retrying."))
			}
		}
		last = cur
	}
}

// notifyBlocked toasts (rate-limited) on new kill-switch blocks.
func (a *App) notifyBlocked() {
	if !a.notify.blocked() {
		return
	}
	a.notify.toast(a.tr("Bağlantı engellendi", "Connection blocked"),
		a.tr("Kill switch bir bağlantıyı engelledi. Ayrıntılar Engellenenler ekranında.",
			"The kill switch blocked a connection. See the Blocked screen for details."))
}

// refreshSettingsCache pulls settings for notification gating.
func (a *App) refreshSettingsCache() {
	var st domain.Settings
	if err := a.call("settings.get", nil, &st); err == nil {
		a.settingsMu.Lock()
		a.settings = st
		a.settingsMu.Unlock()
		a.notify.configure(st.NotificationsEnabled, st.NotifyProxyEvents, st.NotifyBlocked)
	}
}

// tr picks a string by the effective language for toasts/tray.
func (a *App) tr(tr, en string) string {
	a.settingsMu.RLock()
	lang := a.settings.Language
	a.settingsMu.RUnlock()
	if lang == "" {
		lang = domain.Language(a.prefs.Language)
	}
	if lang == domain.LangTR {
		return tr
	}
	return en
}

func (a *App) emit(name string, data any) {
	if a.ctx != nil {
		wruntime.EventsEmit(a.ctx, name, data)
	}
}

// ---------------------------------------------------------------------------
// ipc plumbing
// ---------------------------------------------------------------------------

func (a *App) call(method string, params any, out any) error {
	raw, err := a.cli.Call(method, params, defaultCallTimeout)
	if err != nil {
		return asRPCError(err)
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// bound methods — profiles
// ---------------------------------------------------------------------------

func (a *App) GetProfiles() ([]domain.ProxyProfile, error) {
	var out []domain.ProxyProfile
	return out, a.call("profiles.list", nil, &out)
}

func (a *App) CreateProfile(in appsvc.ProfileInput) (*domain.ProxyProfile, error) {
	var out *domain.ProxyProfile
	return out, a.call("profiles.create", in, &out)
}

func (a *App) UpdateProfile(id string, in appsvc.ProfileInput) (*domain.ProxyProfile, error) {
	var out *domain.ProxyProfile
	return out, a.call("profiles.update", map[string]any{"id": id, "input": in}, &out)
}

func (a *App) DeleteProfile(id string) error {
	return a.call("profiles.delete", map[string]any{"id": id}, nil)
}

func (a *App) SetProfileEnabled(id string, enabled bool) error {
	return a.call("profiles.setEnabled", map[string]any{"id": id, "enabled": enabled}, nil)
}

func (a *App) TestProfile(id string) (appsvc.TestResult, error) {
	var out appsvc.TestResult
	return out, a.call("profiles.test", map[string]any{"id": id}, &out)
}

func (a *App) GetProfileStatuses() ([]domain.ProxyStatus, error) {
	var out []domain.ProxyStatus
	return out, a.call("profiles.statuses", nil, &out)
}

// ---------------------------------------------------------------------------
// bound methods — rules
// ---------------------------------------------------------------------------

func (a *App) GetRules() ([]domain.Rule, error) {
	var out []domain.Rule
	return out, a.call("rules.list", nil, &out)
}

func (a *App) CreateRule(in appsvc.RuleInput) (*appsvc.RuleResult, error) {
	var out *appsvc.RuleResult
	return out, a.call("rules.create", in, &out)
}

func (a *App) UpdateRule(id string, in appsvc.RuleInput) (*appsvc.RuleResult, error) {
	var out *appsvc.RuleResult
	return out, a.call("rules.update", map[string]any{"id": id, "input": in}, &out)
}

func (a *App) DeleteRule(id string) error {
	return a.call("rules.delete", map[string]any{"id": id}, nil)
}

func (a *App) SetRuleEnabled(id string, enabled bool) error {
	return a.call("rules.setEnabled", map[string]any{"id": id, "enabled": enabled}, nil)
}

func (a *App) SetRuleKillSwitch(id string, enabled bool) error {
	return a.call("rules.toggleKillSwitch", map[string]any{"id": id, "enabled": enabled}, nil)
}

func (a *App) ListProcesses() ([]appsvc.ProcessInfo, error) {
	var out []appsvc.ProcessInfo
	return out, a.call("processes.list", nil, &out)
}

// ---------------------------------------------------------------------------
// bound methods — engine / settings
// ---------------------------------------------------------------------------

func (a *App) StartEngine() (appsvc.EngineStatus, error) {
	var out appsvc.EngineStatus
	return out, a.call("engine.start", nil, &out)
}

func (a *App) StopEngine() (appsvc.EngineStatus, error) {
	var out appsvc.EngineStatus
	return out, a.call("engine.stop", nil, &out)
}

func (a *App) EngineStatus() (appsvc.EngineStatus, error) {
	var out appsvc.EngineStatus
	return out, a.call("engine.status", nil, &out)
}

func (a *App) GetSettings() (domain.Settings, error) {
	var out domain.Settings
	err := a.call("settings.get", nil, &out)
	if err == nil {
		a.settingsMu.Lock()
		a.settings = out
		a.settingsMu.Unlock()
		a.notify.configure(out.NotificationsEnabled, out.NotifyProxyEvents, out.NotifyBlocked)
	}
	return out, err
}

func (a *App) UpdateSettings(st domain.Settings) (domain.Settings, error) {
	var out domain.Settings
	err := a.call("settings.update", st, &out)
	if err == nil {
		a.settingsMu.Lock()
		a.settings = st
		a.settingsMu.Unlock()
		a.notify.configure(st.NotificationsEnabled, st.NotifyProxyEvents, st.NotifyBlocked)
	}
	return out, err
}

// ---------------------------------------------------------------------------
// bound methods — stats / data
// ---------------------------------------------------------------------------

func (a *App) GetSnapshot() (domain.LiveSnapshot, error) {
	var out domain.LiveSnapshot
	return out, a.call("stats.snapshot", nil, &out)
}

func (a *App) GetHistory() ([]domain.LiveSnapshot, error) {
	var out []domain.LiveSnapshot
	return out, a.call("stats.history", nil, &out)
}

func (a *App) GetConnections() ([]domain.Connection, error) {
	var out []domain.Connection
	return out, a.call("stats.conns", nil, &out)
}

func (a *App) GetExeStats(from, to string) ([]domain.ExeDailyStats, error) {
	var out []domain.ExeDailyStats
	return out, a.call("stats.exe", map[string]string{"from": from, "to": to}, &out)
}

func (a *App) GetProxyStats(from, to string) ([]domain.ProxyDailyStats, error) {
	var out []domain.ProxyDailyStats
	return out, a.call("stats.proxy", map[string]string{"from": from, "to": to}, &out)
}

func (a *App) ResetStats(exePath string) error {
	return a.call("stats.reset", map[string]string{"exe_path": exePath}, nil)
}

// LastSession returns the previous engine session's persisted snapshot
// (nil when none was recorded yet).
func (a *App) LastSession() (*domain.LastSession, error) {
	var out *domain.LastSession
	return out, a.call("stats.lastSession", nil, &out)
}

// ExportStatsCSV returns the CSV text for daily stats in [from,to].
// kind: "exe" | "proxy" | "all".
func (a *App) ExportStatsCSV(from, to, kind string) (string, error) {
	var out string
	return out, a.call("stats.exportCsv",
		map[string]string{"from": from, "to": to, "kind": kind}, &out)
}

func (a *App) GetBlocked(limit int) ([]domain.BlockedEntry, error) {
	var out []domain.BlockedEntry
	return out, a.call("blocked.list", map[string]int{"limit": limit}, &out)
}

func (a *App) ClearBlocked() error {
	return a.call("blocked.clear", nil, nil)
}

func (a *App) ExportProfiles(includeSecrets bool) ([]appsvc.ExportProfile, error) {
	var out []appsvc.ExportProfile
	return out, a.call("data.export", map[string]bool{"include_secrets": includeSecrets}, &out)
}

func (a *App) ImportProfiles(profiles []appsvc.ExportProfile, overwrite bool) (appsvc.ImportResult, error) {
	var out appsvc.ImportResult
	return out, a.call("data.import", map[string]any{"profiles": profiles, "overwrite": overwrite}, &out)
}

// ---------------------------------------------------------------------------
// bound methods — local helpers
// ---------------------------------------------------------------------------

// PickExeFile opens an .exe file dialog.
func (a *App) PickExeFile() (string, error) {
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   a.tr("Uygulama seçin", "Pick an application"),
		Filters: []wruntime.FileFilter{{DisplayName: "Program (*.exe)", Pattern: "*.exe"}},
	})
}

// PickFolder opens a directory dialog.
func (a *App) PickFolder() (string, error) {
	return wruntime.OpenDirectoryDialog(a.ctx, wruntime.OpenDialogOptions{
		Title: a.tr("Klasör seçin", "Pick a folder"),
	})
}

// PickJSONFile opens a .json file dialog (import).
func (a *App) PickJSONFile() (string, error) {
	return wruntime.OpenFileDialog(a.ctx, wruntime.OpenDialogOptions{
		Title:   a.tr("Dosya seçin", "Pick a file"),
		Filters: []wruntime.FileFilter{{DisplayName: "JSON (*.json)", Pattern: "*.json"}},
	})
}

// PickSaveFile opens a save dialog and returns the chosen path.
func (a *App) PickSaveFile(defaultName, pattern string) (string, error) {
	return wruntime.SaveFileDialog(a.ctx, wruntime.SaveDialogOptions{
		Title:                a.tr("Kaydet", "Save as"),
		DefaultFilename:      defaultName,
		Filters:              []wruntime.FileFilter{{DisplayName: pattern, Pattern: pattern}},
		CanCreateDirectories: true,
	})
}

// PathInfo describes a dropped/picked path for the rules UI.
type PathInfo struct {
	Exists bool   `json:"exists"`
	IsDir  bool   `json:"is_dir"`
	Ext    string `json:"ext"`
	Name   string `json:"name"`
}

func (a *App) PathInfo(path string) PathInfo {
	pi := PathInfo{Name: filepath.Base(path), Ext: strings.ToLower(filepath.Ext(path))}
	if st, err := os.Stat(path); err == nil {
		pi.Exists = true
		pi.IsDir = st.IsDir()
	}
	return pi
}

// ReadTextFile returns the contents of a small text file (import).
func (a *App) ReadTextFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	return string(b), err
}

// WriteTextFile writes a text file (CSV export).
func (a *App) WriteTextFile(path, contents string) error {
	return os.WriteFile(path, []byte(contents), 0o644)
}

// GetExeIcon returns a base64 PNG data-URL for an exe (or a generated
// fallback). Results are cached.
func (a *App) GetExeIcon(path string) string {
	a.iconsMu.Lock()
	defer a.iconsMu.Unlock()
	if icon, ok := a.icons[path]; ok {
		return icon
	}
	icon := guiicon.DataURL(path)
	a.icons[path] = icon
	return icon
}

// ---------------------------------------------------------------------------
// bound methods — service management & app prefs
// ---------------------------------------------------------------------------

// ServiceAvailable reports whether the service pipe answers.
func (a *App) ServiceAvailable() bool { return a.ping() }

// ServiceStatus exposes installed/running/binary-found for the dashboard
// "service unreachable" state.
func (a *App) ServiceStatus() ServiceStatus {
	return queryServiceStatus()
}

// InstallService elevates proxity-service.exe -install via UAC. The service
// binary installs and starts itself.
func (a *App) InstallService() error {
	return installService()
}

// UninstallService elevates proxity-service.exe -uninstall via UAC.
func (a *App) UninstallService() error {
	return uninstallService()
}

// Quit performs a real quit (bypasses close-to-tray).
func (a *App) Quit() {
	a.quitting.Store(true)
	if a.ctx != nil {
		wruntime.Quit(a.ctx)
	}
}

// ShowWindow restores the window from the tray.
func (a *App) ShowWindow() {
	if a.ctx == nil {
		return
	}
	wruntime.WindowUnminimise(a.ctx)
	wruntime.WindowShow(a.ctx)
	wruntime.WindowSetAlwaysOnTop(a.ctx, true)
	wruntime.WindowSetAlwaysOnTop(a.ctx, false)
}

// GetGuiPrefs returns the GUI-local prefs (boot path, offline fallback).
func (a *App) GetGuiPrefs() guiprefs.Prefs { return a.prefs }

// SaveGuiPrefs persists GUI-local prefs and keeps the in-memory copy hot.
func (a *App) SaveGuiPrefs(p guiprefs.Prefs) error {
	if err := guiprefs.Save(p); err != nil {
		return err
	}
	a.prefs = p
	return nil
}

// Connected exposes the last known pipe state for the UI shell.
func (a *App) Connected() bool { return a.connected.Load() }

// SetGuiAutoStart toggles the HKCU Run entry for the GUI.
func (a *App) SetGuiAutoStart(enabled bool) error { return setGuiAutoStart(enabled) }

// GuiAutoStart reports whether the GUI Run entry exists.
func (a *App) GuiAutoStart() bool { return guiAutoStart() }

// GuiVersion returns this binary's build version stamp.
func (a *App) GuiVersion() string { return version.Version }

// ServiceVersion returns the service binary's version from the last
// successful ping ("" while the service is unreachable).
func (a *App) ServiceVersion() string {
	if v, ok := a.svcVersion.Load().(string); ok {
		return v
	}
	return ""
}
