# Security Policy

Proxity controls application traffic through a privileged Windows service. Reports about routing bypasses, privilege boundaries, and credential exposure help protect its users.

## Report a vulnerability privately

Use the repository's [private vulnerability reporting page](https://github.com/Glimor/proxity/security/advisories/new) when available. If reporting is not enabled, use a private contact method listed on the [maintainer's profile](https://github.com/Glimor). Do not publish exploit details in a public issue.

Include:

- Proxity version, Windows build, and whether you used an installer or a source build.
- Relevant settings, including rule action, kill-switch mode, and Secure DNS state.
- Reproduction steps or a minimal proof of concept, expected behavior, and observed impact.
- Sanitized logs, plus your preferred contact and credit/disclosure preferences.

Do not send live credentials or unredacted proxy exports. Fix timing depends on severity and available maintainer capacity; acknowledgement and release dates are not guaranteed.

## Supported versions

Security fixes target the latest published release and the current development branch. Older releases do not have a separate maintained patch line. Confirm an issue against the latest release when practical, and state the version you actually tested.

## Areas of particular interest

| Area | Examples |
| --- | --- |
| Routing and kill switches | Protected traffic escaping when the proxy, engine, or service is unavailable; unexpected WFP permits; IPv4/IPv6 or UDP bypasses. |
| GUI-to-service boundary | Named-pipe access, request validation, unsafe privileged operations, or local privilege escalation. |
| Credentials | DPAPI misuse, unintended plaintext in logs or exports, and unauthorized access to stored secrets. |
| Secure DNS | Plaintext DNS escaping while managed resolution is enabled, or adapter settings not being restored correctly. |

## Security model and limits

- The engine service runs as **SYSTEM**. Installing or removing it requires administrator access; the GUI can run without elevation.
- The named pipe grants access to **SYSTEM, Administrators, and interactive users** on the same machine. This allows local users to control Proxity through the GUI; it is not a per-user isolation boundary. The service must validate incoming requests.
- Kill-switch protection is configurable. Persistent WFP BLOCK filters are paired with dynamic-session PERMIT filters. The configuration, rule coverage, LAN exemptions, and proxy health determine what is protected.
- Stored proxy passwords are encrypted with **machine-scope Windows DPAPI**. This protects storage but does not isolate secrets from administrators or all other users of the same machine. GUI credentials pass through local IPC; optional secret exports contain machine-bound encrypted ciphertext and must be handled as secrets.
- Proxity routes **TCP** through HTTP CONNECT, HTTPS CONNECT, SOCKS4(a), or SOCKS5 and does not relay UDP. Secure DNS is optional and does not make every application's DNS behavior equivalent.
- HTTPS proxy connections verify the proxy certificate and host name using system
  trust roots. HTTP proxy Basic credentials and SOCKS credentials are not encrypted
  by those protocols; HTTPS proxies add TLS to the proxy transport.
- SOCKS4 supports a user ID only, without password authentication or IPv6 destinations.
- Release binaries are currently **unsigned**. Obtain them from the project's [Releases](https://github.com/Glimor/proxity/releases) page and compare published checksums. Checksums detect a mismatch but do not replace publisher signing.

For normal setup and troubleshooting, see the [user guide](docs/user-guide.md).
