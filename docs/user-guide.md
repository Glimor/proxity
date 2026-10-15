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
| **Profile** | An HTTP, HTTPS, SOCKS4 or SOCKS5 proxy with a type, host, port, and optional credentials. The password is encrypted with DPAPI and tied to the machine. |
| **Rule** | An exe or folder, an action (Proxy / Direct / Block), and an optional kill switch. Folder rules can include subfolders. |
| **Engine** | Captures outgoing TCP SYN packets with WinDivert, then reflects and proxies them according to the matching rule. |
| **Kill switch** | WFP filters that block traffic if the engine stops. Can be configured per rule or globally. |

## Proxy profiles

In the setup wizard or **Proxies → Add Proxy / Edit**, choose **Proxy type**,
then enter the host (without a URL scheme) and the provider's port. The card shows
the saved type. **Test Connection** uses that type, as do background health checks
and Secure DNS when this profile is selected.

- **HTTP**: CONNECT tunneling, with optional Basic username/password authentication.
- **HTTPS**: CONNECT over TLS to the proxy, with optional Basic authentication.
  Use a host matching a certificate trusted by Windows; certificate checks cannot
  be disabled in the UI.
- **SOCKS4**: optional **User ID**; the password field is hidden. Switching an
  existing profile to SOCKS4 clears its stored password. IPv4 is supported;
  names require SOCKS4a support on the server. IPv6 destinations are unsupported.
- **SOCKS5**: optional username/password, with IPv4, IPv6 and proxy-side name resolution.

All types relay TCP only. HTTP(S) servers must allow CONNECT to the destination
port. The connection test probes port 443; exit-IP lookup is best effort and may
be unavailable when the server disallows port 80. Secure DNS uses the configured
resolver port, which the proxy must also allow.

Existing profiles are automatically migrated to SOCKS5. Profile exports include
`protocol` (`http`, `https`, `socks4`, or `socks5`); older imports without it use
SOCKS5. Unknown types and SOCKS4 profiles containing passwords are rejected.

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

- **Start service with Windows**: controls the installed service's startup
  mode (Automatic/Manual). Turning it off does not uninstall the service.
- **Global kill switch**: if the engine stops, internet access is blocked for
  the entire machine. LAN access can be allowed separately.
- **Secure DNS**: changes adapter DNS to 127.0.0.1. Queries are resolved
  through a proxy so plaintext DNS does not leave the machine. You can choose
  the upstream and the profile used for DNS.
- **Notifications**: alerts for blocked connections and proxy
  disconnections/reconnections.
- **Import/export**: profiles can be transferred as JSON. Including passwords
  is optional; store exported files securely.

## Window and tray

The engine starts automatically when the background service starts. Check
its status on the Dashboard; use that screen to explicitly stop or start it.
Closing the window hides it in the tray. Right-click the Proxity tray icon
for **Show Proxity** or **Quit Proxity**. Quitting closes the GUI and tray;
the background service keeps running.

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
