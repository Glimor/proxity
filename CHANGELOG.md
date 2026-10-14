# Changelog

All notable changes to Proxity are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/);
versioning follows [SemVer](https://semver.org/).

## [Unreleased]

### Fixed

- Deleting the ignored build directory no longer removes required source
  assets. Branding and packaging templates live outside build/, and tracked
  build scripts recreate all resources for clean checkouts and release builds.

- Desktop notifications identify Proxity instead of using the notification
  library's default app ID; cached notification branding refreshes on updates.
- The window, installer, tray, frontend and notifications use the supplied
  Proxity logo. The tray no longer offers an Engine checkbox; service-owned
  engine startup remains automatic.
- The installer packages the background service and WinDivert files again,
  starts the service, and removes it before deleting service data on uninstall.

- Empty profile/rule/stat lists no longer crash GUI menus after skipping setup.
- Setup selects the newly saved proxy, applies corrected credentials after a
  connection test, and stays on the form when rule or preference saving fails.
- Proxy names are optional; authentication fields are visible on creation.
- Reconnecting refreshes screen data; empty connection events clear stale rows
  and blocked-log event handlers are removed when leaving the screen.
- Cancelling a conflicting rule edit restores the original rule. Proxy tests
  and saves cannot run simultaneously from the same form.
- Disabling service autostart selects manual startup without uninstalling it.
- SOCKS5 handshakes obey timeouts and cancellation; profile edits restart health
  checks using the updated credentials.
- Failed engine starts can be retried. Start RPCs report the resulting status,
  and GUI disconnections do not cancel the engine's service lifetime.
- Disconnecting an IPC client cancels pending handlers before draining them.

### Added

- User-focused README with real-interface demo screenshots, repeatable
  screenshot/branding scripts, and updated contribution and security guidance.

- Browser regression tests for setup, every GUI menu, authenticated proxies,
  reconnects, error handling, rule cancellation and service startup settings.
- Go regression coverage for bridge responses, empty lists, SOCKS5 timeouts,
  health-check updates, failed starts and IPC disconnects.

## [0.1.0] — 2026-10-14

First public release.

### Added

- Per-application proxy routing: exe/folder rules with proxy / direct /
  block actions over SOCKS5 (with optional username/password auth,
  DPAPI-encrypted at rest).
- Fail-closed kill switch: persistent WFP BLOCK filters plus
  dynamic-session PERMITs, per-app and global modes, LAN/loopback
  exemptions, health-gated TCP permits.
- Self recv-accept permit at `ALE_AUTH_RECV_ACCEPT` so the transparent
  redirect's reflected SYNs survive the default inbound-block policy.
- UDP-leak blocking: proxy rules without a kill switch block remote UDP
  at WFP level (UDP cannot be proxied; LAN/loopback stays open).
- Transparent TCP redirect via WinDivert packet reflection (natID in the
  reflected source port), wildcard listener on both stacks.
- Secure DNS: loopback forwarder at 127.0.0.1, adapter DNS takeover,
  remote-resolution domain cache for SOCKS dials, port-53 egress block.
- Wails GUI (Vue 3): dashboard, proxies, rules, live connections, stats
  with CSV export, blocked log, settings, first-run wizard; system tray.
- Windows service (`ProxityEngine`) with SCM install/uninstall, recovery
  actions, power-resume and network-change proxy rechecks.
- Named-pipe JSON-RPC IPC with reconnecting GUI client and event stream;
  ping now carries the service build version for GUI/service mismatch
  warnings.
- Build versioning (`internal/version`, `-ldflags -X`), NSIS installer
  (service install/remove + WinDivert driver staging), `build.ps1`.
- CI: gofmt/vet/test, frontend build, wails build, govulncheck;
  release workflow publishing installer + SHA256 checksums.

### Notes

- Binaries and installer are unsigned — SmartScreen will warn.
- Uninstall deletes `%ProgramData%\Proxity` (rules, profiles, stats,
  logs). Export profiles first if you want to keep them.
