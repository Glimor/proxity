# Proxity dev guide

## Repo layout

```
main.go app.go service.go tray.go notify.go branding.go  Wails GUI binary (proxity.exe)
cmd/proxity-service/                          service binary (console/-engine/-install/-uninstall)
internal/
  appsvc/        service-facing feature layer (rules/profiles/settings/stats over IPC)
  dnsfwd/        loopback DNS forwarder + adapter takeover + domain cache
  domain/        shared model: rules, profiles, settings, stats, errors
  engine/        WinDivert capture, flow table, reflect redirect, listener, relay, health
  guiicon/       exe icon → data URL
  guiprefs/      GUI-local prefs (json under user profile)
  ipc/           named-pipe JSON-RPC client/server (go-winio)
  killswitch/    desired-state → WFP reconcile (pure, unit-testable)
  logging/       slog file+console init
  netpkt/        packet parse/rewrite helpers for the reflect trick
  netwatch/      interface/address change watcher
  ports/         store interfaces (sqlite implements)
  procmon/       TCP-table process attribution
  secrets/       DPAPI machine-scope cipher
  proxy/         protocol dispatcher, HTTP(S) CONNECT and SOCKS4(a) clients
  socks5/        SOCKS5 client (no-auth + user/pass)
  storage/sqlite/ schema, migrations, repos
  svccfg/        SCM start-type helper (appsvc → service start mode)
  version/       build stamp (-X …/version.Version)
  wfp/           minimal fwpuclnt.dll binding (from MIT WireGuard code)
  windivert/     minimal WinDivert.dll binding + driver discovery
frontend/        Vue3 + TS + Vite + vue-i18n (English)
tests/           manual WFP integration harness (PROXITY_INTEG=1)
assets/branding/          canonical logo and embedded Windows ICO
packaging/windows/       versioned Windows metadata, manifest and NSIS template
scripts/                 build, resource preparation and branding scripts
build/                   ignored, disposable resources and binaries
third_party/windivert/    WinDivert.dll + WinDivert64.sys (LGPL)
```

## Running locally

Use the Go version in `go.mod`, Node.js 22.12+ and Wails CLI v2.16.0.

```powershell
# service in console mode (elevated shell, sees logs live)
go build -o build\bin\proxity-service.exe .\cmd\proxity-service
copy third_party\windivert\WinDivert.* build\bin\
.\build\bin\proxity-service.exe -console

# or engine-only harness (no DB writes beyond config load)
.\build\bin\proxity-service.exe -engine

# GUI
wails dev        # or: wails build → build\bin\proxity.exe
```

The GUI finds `proxity-service.exe` next to itself or in
`..\proxity-service.exe` (see `findServiceBinary` in service.go) — the
build/bin layout already satisfies this.

## Version stamping

Both binaries read `internal/version.Version` ("dev" default). Release
builds stamp it: `scripts/build.ps1 -Version X.Y.Z`, which passes
`-ldflags "-X github.com/Glimor/proxity/internal/version.Version=X.Y.Z"`
to `go build` (service) and `wails build` (GUI). The service logs it at
startup, the GUI shows it in Settings (and pings it for mismatch checks).

## Test strategy

Frontend regression tests use a headless browser with mocked Wails bindings;
they never install a service or modify system routing:

```powershell
cd frontend
npm ci
npx playwright install chromium
npm test
# Or use an installed Edge instead of downloading Chromium:
$env:PLAYWRIGHT_CHANNEL = 'msedge'
npm test
```

These cover first-run skip and all menus with null lists, all four proxy types, credentials
and corrections after a test, failed rule/preference writes, conflicting rule
edit cancellation, reconnects, event listener cleanup and service startup mode.
Run `npm run typecheck` and `npm run build` alongside them. For Go checks without
driver capture, use `go test -short ./...` from the repository root.

Quick proxy-only checks (local fake servers, no internet or driver required):

```powershell
go test -count=1 ./internal/proxy ./internal/socks5
go test -short ./internal/storage/sqlite ./internal/appsvc ./internal/engine ./internal/dnsfwd
```

`domain.ProxyProfile.Protocol` is persisted by migration `0002_proxy_protocol.sql`.
An empty protocol maps to SOCKS5 for legacy callers/imports. All engine, health,
and Secure DNS paths use `proxy.Dial`. Protocol edits restart health probes.
HTTPS uses HTTP/1.1 CONNECT over TLS 1.2+ with system certificate verification;
HTTP(S) supports Basic authentication, SOCKS4 supports user IDs and SOCKS4a names,
and SOCKS5 supports RFC 1928/1929. Tests use a local trusted certificate pool for
TLS success and verify rejection of untrusted and mismatched certificates.
UI tests cover creation, editing, type persistence and mocked connection tests
for each protocol. Storage/service tests cover migration and export/import.
Protocol references: [HTTP CONNECT (RFC 9110)](https://www.rfc-editor.org/rfc/rfc9110.html#section-9.3.6),
[SOCKS4 specification](https://www.openssh.org/txt/socks4.protocol), and
[SOCKS5 (RFC 1928)](https://www.rfc-editor.org/rfc/rfc1928.html).

| layer                | style                                                        |
|----------------------|--------------------------------------------------------------|
| killswitch/wfp       | pure diff/reconcile unit tests against a fake Backend        |
| appsvc               | sqlite-backed service tests, fake engine + fake kill switch   |
| engine               | unit tests + `openListeners` smoke; reflect path is manual    |
| proxy / socks5       | local fake servers: tunnels, authentication, TLS trust/host checks, cancellation, timeouts and rejection replies |
| storage/sqlite       | real DB in t.TempDir                                          |
| `tests/`             | **manual**: real WFP backend, `PROXITY_INTEG=1`, admin, service STOPPED — OpenBackend resets the shared sublayer |
| `internal/engine/load_test.go` | **manual**: `PROXITY_INTEG=1`; injects flows + binds source port = natID to exercise listener→SOCKS5→relay (no driver needed) |

CI runs `gofmt -l`, `go vet`, `go test ./...` (elevated-only tests
self-skip), `npm ci && npm run build`, and `wails build` via
`.github/workflows/ci.yml`. Releases (tag `v*`) run `scripts/build.ps1` and
publish the installer + checksums via `release.yml`.

## Branding and documentation images

`assets/branding/logo.png` is the canonical logo. Run
`powershell -ExecutionPolicy Bypass -File scripts/update-branding.ps1` to
regenerate `assets/branding/icon.ico` and `frontend/public/logo.png`.
`scripts/build.ps1` also runs this step. The GUI embeds both persistent assets;
the tray uses the ICO and Windows notifications cache the embedded PNG in
`%LocalAppData%\Proxity\branding\logo.png`. Notification identity and the
taskbar AppUserModelID are `Proxity`.

Generate README screenshots from `frontend/` with
`node scripts/capture-screenshots.mjs` (set `PLAYWRIGHT_CHANNEL=msedge`
for installed Edge). The script renders the real UI with deterministic
demo data and saves images in `docs/screenshots/`; visually review them
before committing. No service or driver is started.

The engine starts automatically in `runServiceCore`; the tray only offers
Show Proxity and Quit Proxity. Closing or quitting the GUI does not stop
the service. Use the Dashboard to explicitly stop or restart the engine.

## Gotchas

- `build/` contains only disposable files. `scripts/prepare-build.ps1` stages
  branding and templates from `assets/` and `packaging/`; the Windows Wails
  pre-build hook also runs it. Root `build.ps1`/`build.bat` are local wrappers;
  CI and clean checkouts use the tracked `scripts/build.ps1` directly.
- `wails build -clean` deletes `build/bin` — scripts/build.ps1 stages the service
  + driver into `build/bin` *before* the wails step, so `-clean` is not
  used. If you add it back, stage after cleaning instead.
- `wails_tools.nsh` is generated by Wails. Edit the source template at
  `packaging/windows/installer/project.nsi`, not the staged copy under build/.
- `App.d.ts`/`App.js` under `frontend/wailsjs` are regenerated by wails —
  don't hand-edit.

## Manual GitHub releases

After `.github/workflows/release.yml` reaches the default branch, open
**Actions → release → Run workflow**, select `main`, and enter a version without
`v` (for example `0.2.0` or `0.2.0-rc.1`). The manual form defaults to `0.2.0`;
increment it for each new release. The selected commit must be current main with
successful CI and CodeQL checks. Manual releases retain these checks and the
pinned Actions, permissions, license bundle and draft-publication safeguards.

The workflow sets application and Windows installer versions, then publishes the
installer, distribution ZIP and checksums under the matching `v` tag. A missing
tag is created at that main commit. An existing tag must point to the same commit;
otherwise the run fails. Numeric `vX.Y.Z` and `vX.Y.Z-rc.N` pushes also trigger
releases. Windows version components must each be at most 65535; RC builds retain
the full application version and use the numeric core for Windows metadata.
