# Contributing to Proxity

Thanks for your interest! Proxity is GPL-3.0; contributions are licensed
under the same terms.

## Getting set up

- Go (version in `go.mod`), Node.js 20+, Wails CLI
  (`go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0`).
- Windows 10/11. Most unit tests run unelevated; WFP/WinDivert paths need
  an elevated shell.
- `wails dev` for live-reload GUI work; `proxity-service.exe -console`
  for the service side.

See [docs/dev.md](docs/dev.md) for the repo layout and test strategy.

## Ground rules

- **Pure Go, `CGO_ENABLED=0`** — no cgo, no new native deps. Kernel access
  goes through our own syscall bindings (`internal/wfp`,
  `internal/windivert`, both adapted from MIT-licensed WireGuard code —
  keep the NOTICE headers).
- **Fail-closed is sacred.** Any change to `internal/killswitch` or
  `internal/wfp` must preserve the invariant: persistent BLOCK filters
  survive process death; PERMIT filters are dynamic-session only. Add a
  unit test for every new desired-filter combination in
  `computeDesired`.
- `gofmt` clean, `go vet` clean, `go test ./...` green.
- Frontend: TypeScript + Vue 3 composition API, i18n keys in
  `frontend/src/i18n/{tr,en}.json` — add **both** languages for new UI
  strings (Turkish is the primary audience).
- No new dependencies without a strong reason — open an issue first.

## Commits / PRs

- Small, focused commits; imperative subject lines
  (`killswitch: recv-accept permit + udp block`).
- PRs: describe the user-visible behavior change and how you verified it
  (tests run, manual checks). Screenshots for UI changes.
- Integration tests are env-guarded (`PROXITY_INTEG=1`) — describe manual
  verification steps in the PR when you can't run them.

## Reporting bugs

Open a GitHub issue with: Windows build (`winver`), Proxity version
(Settings → about line), `%ProgramData%\Proxity\logs` excerpt, and
repro steps. Security issues: see [SECURITY.md](SECURITY.md) — **do not**
open a public issue.
