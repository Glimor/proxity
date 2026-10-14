# Changelog

All notable changes to Proxity are documented here.
Format follows [Keep a Changelog](https://keepachangelog.com/);
versioning follows [SemVer](https://semver.org/).

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
