# Proxity architecture

Proxity is two processes plus a kernel driver:

```
┌────────────────────┐      named pipe \\.\pipe\proxity       ┌──────────────────────────┐
│ proxity.exe        │  JSON-RPC calls + event broadcast      │ proxity-service.exe      │
│ Wails v2 GUI       │ ◄───────────────────────────────────►  │ Windows service (SYSTEM) │
│ Vue3/TS, tray      │  ping, profiles.*, rules.*, engine.*,  │                          │
│                    │  settings.*, stats.*, blocked.*        │                          │
└────────────────────┘                                        │  appsvc                  │
                                                              │   ├─ sqlite (ProgramData)│
                                                              │   ├─ killswitch → WFP    │
                                                              │   ├─ dnsfwd (127.0.0.1)  │
                                                              │   └─ engine → WinDivert  │
                                                              └──────────────────────────┘
```

The GUI is a pure client: it owns only tray/notifications, GUI-local prefs
(`guiprefs`) and exe-icon extraction. Everything else — rules, profiles,
settings, stats, engine control — lives in `appsvc` and is reachable over
JSON-RPC on a Windows named pipe (`internal/ipc`, `go-winio`).

## The engine (internal/engine)

### Capture

WinDivert opens a NETWORK-layer handle with a filter matching outbound TCP
SYNs (`captureFilter`). For every packet the engine resolves the owning
process (`procmon`, TCP-table snapshot + notifications), folds the exe
path and runs rule matching (`domain.MatchRule`).

### Decisions

- **direct** → reinject untouched.
- **block** → drop; logged to the blocked table (deduped per 4-tuple).
- **proxy** → register a *flow*, stamp it with a 16-bit `natID`, and
  **reflect** the packet: instead of forwarding the SYN to its real
  destination, the engine rewrites it into an inbound packet
  `src = originalDst : natID`, `dst = originalSrc : 37890`.

### The reflect trick

`37890` (`ProxyPort`) is Proxity's wildcard listener. The reflected packet
is addressed to the app's own real source IP — always a configured local
address — so the kernel delivers it to our listener on both IPv4 and IPv6
stacks. When `Accept` returns the connection, `RemoteAddr` reads as
`originalDst:natID`: the natID maps straight back to the flow, which knows
the true destination, the process and the rule. The engine then SOCKS5-
CONNECTs to the original destination through the rule's proxy and relays
both directions. To the app, the connection looks completely ordinary —
it thinks it is talking to the real server.

Caveat: at ALE layer `FWPM_LAYER_ALE_AUTH_RECV_ACCEPT`, reflected SYNs
look like unsolicited inbound connections to the service binary. The
default inbound-block firewall policy can drop them, so the kill switch
always installs a persistent self-appID PERMIT at that layer
(`self|recvaccept|v4/v6`, `WeightPermitSelf`) whenever `SelfPath` is known.

### Health & failure modes

`hc` health-checks proxies; the engine reports `CaptureAlive` only while
the WinDivert loop and listeners live. On any death it calls
`OnCaptureDead` → `appsvc` → `killswitch.SetHealthy(false)`.

## The kill switch (internal/killswitch + internal/wfp)

Fail-closed is a *session* property, not polling logic:

| kind        | WFP flag      | lifetime                      |
|-------------|---------------|-------------------------------|
| BLOCK       | PERSISTENT    | survives crash/reboot         |
| PERMIT      | (none)        | dies with the dynamic session |

`SetHealthy(false)` reopens the WFP session — every dynamic permit vanishes
instantly, no enumeration needed. Persistent blocks remain. Weight order
inside `ALE_AUTH_CONNECT`: self(4000) > healthy(3000) > DNS permits(2600)
> DNS block(2500) > local(2000) > block(1000).

Filter derivation is a pure function (`computeDesired`) — reconciled
transactionally against an installed-set map, fully unit-tested with a
fake backend (`killswitch_test.go`). `internal/wfp` is a minimal pure-Go
`fwpuclnt.dll` binding adapted from MIT-licensed WireGuard code.

Per-app policy shapes:

| rule                        | filters                                              |
|-----------------------------|------------------------------------------------------|
| proxy + kill switch         | block-all + local permit + healthy TCP permit        |
| proxy, no kill switch       | UDP-only block + local permit (TCP untouched)        |
| block                       | block-all + local permit                             |
| global kill switch          | same shapes with appID=nil, plus self/DNS permits    |

## Secure DNS (internal/dnsfwd)

When enabled: a loopback UDP/TCP forwarder at 127.0.0.1:53 receives all
queries (adapter DNS is repointed at loopback), forwards upstream — via a
chosen proxy or directly — and caches name→IP answers. The engine consults
that cache (`DomainLookup`) so the SOCKS5 dial can carry the *domain name*
and the proxy resolves remotely: no plaintext DNS leaves the machine. A
persistent port-53 egress block (loopback-exempt) traps anything bypassing
the forwarder.

## Storage & secrets

`modernc.org/sqlite` (pure Go) at `%ProgramData%\Proxity\proxity.db`
(fallback `%LocalAppData%`). Corrupt DBs are moved aside automatically.
Passwords are DPAPI-encrypted with machine scope (`internal/secrets`).
