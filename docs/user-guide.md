# Proxity User Guide

## Installation

1. Run `Proxity-amd64-installer.exe` and approve the UAC prompt. The installer
   copies `proxity.exe` (the GUI), `proxity-service.exe` (the background
   service), and the WinDivert driver files to
   `%ProgramFiles%\Proxity\Proxity`, then installs and starts the
   `ProxityEngine` service.
2. The first-run wizard helps you configure the app.
3. Check the service status on the Dashboard. If the service is unreachable,
   the "Install service" button runs `-install` with UAC approval.

> A SmartScreen warning is expected because the builds are unsigned.

## Key concepts

| Concept | Meaning |
|---------|---------|
| **Profile** | A SOCKS5 proxy with a host, port, and optional username/password. The password is encrypted with DPAPI and tied to the machine. |
| **Rule** | An exe or folder, an action (Proxy / Direct / Block), and an optional kill switch. Folder rules can include subfolders. |
| **Engine** | Captures outgoing TCP SYN packets with WinDivert, then reflects and proxies them according to the matching rule. |
| **Kill switch** | WFP filters that block traffic if the engine stops. Can be configured per rule or globally. |

## Rules

- **Proxy**: connections use the selected profile. If the kill switch is
  enabled, the application cannot reach the internet if the engine or proxy
  stops. If disabled, TCP continues directly, but **remote UDP is still
  blocked** because it cannot be proxied.
- **Direct**: behaves like traffic without a matching rule and is useful for
  applications that should explicitly bypass proxies.
- **Block**: the application cannot reach the internet (LAN and loopback
  remain allowed).

If multiple rules target the same path, Proxity warns about the conflict.
The most specific or highest-priority matching rule takes precedence.

## Settings

- **Start service with Windows**: installs or uninstalls the service and
  controls its startup mode (Automatic/Manual).
- **Global kill switch**: if the engine stops, internet access is blocked for
  the entire machine. LAN access can be allowed separately.
- **Secure DNS**: changes adapter DNS to 127.0.0.1. Queries are resolved
  through a proxy so plaintext DNS does not leave the machine. You can choose
  the upstream and the profile used for DNS.
- **Notifications**: alerts for blocked connections and proxy
  disconnections/reconnections.
- **Import/export**: profiles can be transferred as JSON. Including passwords
  is optional; store exported files securely.

## Uninstall

Use `uninstall.exe` or Windows Settings > Apps. The uninstaller:

- Runs `proxity-service.exe -uninstall`, stops and removes the service,
  removes all WFP filters and the provider, and restores adapter DNS settings.
- **Deletes `%ProgramData%\Proxity`** (the database, rules, profiles,
  statistics, and logs). Export profiles first if you want to keep them.

## Troubleshooting

- **Service unreachable**: use the Dashboard recovery prompt to reinstall;
  check the `ProxityEngine` status in `services.msc`.
- **An application is not using its proxy**: verify that the rule path matches
  the exe's full path (for example, use the actual exe rather than an
  updater running under AppData). Rules affect new connections; restart the
  application to re-establish its connections.
- **Logs**: `%ProgramData%\Proxity\logs`. Set the log level to `debug` in
  Settings for more detail.
- **Blocked screen**: shows the live list of kill-switch blocks.
