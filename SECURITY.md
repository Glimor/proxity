# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security problems.
Report privately to the maintainer (see the repository profile for
contact — e.g. GitHub private vulnerability reporting / "Security" tab
→ "Report a vulnerability").

Include: affected version, attack scenario, repro steps or PoC, and
whether you need coordinated disclosure. Expect an acknowledgement within
a few days; we'll agree on a fix timeline and credit you in the release
notes unless you prefer anonymity.

## Scope notes

Things we especially care about:

- **Kill-switch bypasses** — any way a protected app's traffic reaches the
  network while the engine is dead or the proxy is down (WFP filter logic,
  `internal/killswitch`, `internal/wfp`).
- **Local privilege escalation** — the GUI↔service named pipe
  (`internal/ipc`) is a SYSTEM attack surface; request validation and
  auth-model bugs matter.
- **Secrets** — proxy passwords are DPAPI-encrypted (machine scope) and
  must never appear in logs, exports without explicit opt-in, or IPC
  payloads.
- **DNS leaks** — plaintext DNS escaping while secure-DNS mode is on.

## Security model (short)

- The engine service runs as **SYSTEM**; only administrators can install
  or reconfigure it.
- WFP filters: persistent BLOCK + dynamic-session PERMIT — a dead process
  means closed, not open.
- The IPC pipe accepts same-machine clients; the service treats all
  payloads as untrusted.
- Binaries are currently **unsigned**; the installer requires admin by
  design.
