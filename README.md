# Proxity

Proxity is a per-application proxy-routing engine for Windows. Define rules
to choose which SOCKS5 proxy each program uses; all other traffic continues
through its normal route.

**Platform:** Windows 10/11 x64 · **License:** GPL-3.0 · **Implementation:**
pure Go (`CGO_ENABLED=0`; no native dependencies beyond the bundled WinDivert
driver)

> Screenshots coming soon (`docs/screenshots/`).

## Features

- **Per-application routing** — exe or folder rules can proxy, allow directly,
  or block traffic. Proxity does not change system-wide proxy settings.
- **Fail-closed kill switch** — persistent WFP BLOCK filters immediately stop
  protected applications from reaching the internet if the engine stops. The
  global kill switch can block internet access for the whole machine (LAN
  access can remain enabled).
- **UDP leak protection** — because UDP cannot be proxied, remote UDP is
  automatically blocked for proxy rules without a kill switch; TCP is
  unaffected.
- **Secure DNS** — a local resolver at 127.0.0.1 sends queries through a proxy
  so plaintext DNS does not leave the machine.
- **No leaks outside rules** — a persistent RECV_ACCEPT permit for the
  service allows reflected SYN packets; if the service crashes, dynamic
  permits are removed while block filters remain.
- **Statistics** — live charts, active connections, a blocked-connection log,
  daily per-executable and per-proxy totals, CSV export, and 400-day retention.
- **Tray app and setup wizard** — a three-step first-run setup, with dark and
  light themes.

## Installation

Download and run `Proxity-amd64-installer.exe`. Installation requires
**administrator privileges** because it registers the `ProxityEngine` Windows
service and installs the WinDivert kernel driver; these operations require
elevation.

Because the build is unsigned, SmartScreen may show a warning. Select
"More info" and then "Run anyway" to continue.

To uninstall, use Settings > Apps or run `uninstall.exe` from the install
folder. The uninstaller stops the service, removes WFP filters, and restores
DNS settings. It also deletes **`%ProgramData%\Proxity`** (rules, proxies, and
statistics), so export proxies from Settings > Proxies first if you want to
keep them.

## Quick start (3 steps)

1. **Add a proxy** — open Proxies > Add Proxy and enter the SOCKS5 address and
   port. Add a username and password if required; the password is encrypted
   with DPAPI.
2. **Add an application rule** — open Applications > Add App, choose an exe or
   folder, set the action to "Proxy", and select a profile. You can optionally
   enable the kill switch.
3. **Start the engine** — use the control on the Dashboard. The selected
   application will use the proxy; other applications continue to connect
   normally.

## Architecture (overview)

```
┌──────────────┐   named pipe (JSON-RPC)   ┌────────────────────────┐
│ proxity.exe  │ ════════════════════════► │ proxity-service.exe    │
│ Wails GUI    │                           │ (SYSTEM, SCM service)  │
│ Vue3 + tray  │                           │                        │
└──────────────┘                           │  appsvc ── sqlite DB   │
                                           │    │                   │
                                           │    ├─► killswitch ─►   │
                                           │    │     WFP (fwpuclnt)│
                                           │    │  persistent BLOCK │
                                           │    │  dynamic PERMIT   │
                                           │    │                   │
                                           │    └─► engine ──►      │
                                           │      WinDivert capture │
                                           │      SYN reflection -> │
                                           │      127.0.0.1:37890   │
                                           │      SOCKS5 dial ->    │
                                           │      upstream relay    │
                                           └────────────────────────┘
```

See [docs/architecture.md](docs/architecture.md),
[docs/user-guide.md](docs/user-guide.md), and [docs/dev.md](docs/dev.md).

## Building from source

Requirements: Go (the version in `go.mod`), Node.js 20+, the Wails CLI
(`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`), and NSIS for
the installer (`winget install NSIS.NSIS`).

```powershell
# One command builds the service, GUI, and NSIS installer into build\bin:
powershell -ExecutionPolicy Bypass -File build.ps1 -Version 0.1.0
```

Build individual components for development:

```powershell
# Frontend
cd frontend; npm ci; npm run build; cd ..

# Service (use an administrator console to run it in console mode)
go build -o build\bin\proxity-service.exe .\cmd\proxity-service
proxity-service.exe -console      # or -engine to run only the engine

# GUI
wails build            # build\bin\proxity.exe
wails dev              # hot reload
```

Run tests with `go test ./...`. Administrator/integration tests are enabled
with the `PROXITY_INTEG=1` environment variable (see `tests/` and
`internal/engine/load_test.go`).

## License and acknowledgments

- **Proxity**: [GPL-3.0](LICENSE)
- **WinDivert** (driver in `third_party/windivert/`): LGPL —
  [basil00/WinDivert](https://github.com/basil00/WinDivert); see
  `third_party/windivert/LICENSE`.
- The bindings in `internal/wfp` and `internal/windivert` are adapted from
  the MIT-licensed WireGuard for Windows project (see the NOTICE headers in
  those files).
