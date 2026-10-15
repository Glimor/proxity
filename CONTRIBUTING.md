# Contributing to Proxity

Help make per-application routing easier to use and more reliable. Bug reports, documentation improvements, and focused code changes are welcome. Contributions are licensed under the project's [GPL-3.0 license](LICENSE).

## Set up your workspace

Use Windows 10/11 x64, the Go version declared in `go.mod`, Node.js 22.12+ (required by the current Vite version), and Wails CLI v2.16.0:

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
cd frontend
npm ci
npm run build
cd ..
wails dev
```

The GUI connects to the separate Proxity service. See [docs/dev.md](docs/dev.md) for console-mode service setup, the repository layout, and release builds. Real WFP/WinDivert checks need administrator access; the browser tests use mocked bindings.

## Verify your change

Run the checks relevant to your work:

```powershell
go vet ./...
go test -short ./...
cd frontend
npm run typecheck
npm run build
$env:PLAYWRIGHT_CHANNEL = 'msedge'
npm test
```

For Chromium instead of installed Edge, run `npx playwright install chromium` and leave `PLAYWRIGHT_CHANNEL` unset. Driver and integration checks are separately guarded by `PROXITY_INTEG=1`; follow the developer guide before running them because they can modify routing and WFP filters.

## Implementation expectations

- Keep Go code `gofmt` clean and compatible with `CGO_ENABLED=0`. Kernel access uses the existing syscall bindings. Preserve third-party NOTICE headers.
- Preserve fail-closed behavior: persistent BLOCK filters survive process death; PERMIT filters belong to a dynamic session. Cover new `computeDesired` filter combinations with meaningful tests.
- Use TypeScript and the Vue 3 composition API. Put new UI strings in `frontend/src/i18n/en.json` and preserve both light and dark themes.
- Keep dependencies justified and changes focused. Discuss substantial new dependencies or architecture changes in an issue first.
- Protect credentials and personal connection data in logs, fixtures, screenshots, and issue reports.

## Branding and screenshots

`assets/branding/logo.png` is the canonical Proxity logo. The executable, installer, tray, notifications, and frontend must use this branding. To refresh the frontend copy and `assets/branding/icon.ico` after replacing it:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/update-branding.ps1
```

The release build also runs this script. `scripts/prepare-build.ps1` copies the
logo and the versioned `packaging/windows/` templates into the disposable
`build/` directory. Keep `assets/`, `packaging/`, and `scripts/` in Git;
the build must work without pre-existing outputs. Use `scripts/build.ps1`
for a release build. Application icons shown in rule/connection rows belong
to the selected applications and remain separate from Proxity's brand.

README screenshots are generated from the actual Vue interface with deterministic demo data:

```powershell
cd frontend
$env:PLAYWRIGHT_CHANNEL = 'msedge'
node scripts/capture-screenshots.mjs
```

The script captures the dashboard in both light and dark themes, plus rules
and proxies. Review `docs/screenshots/*.png` visually before committing.
Do not use real credentials, private endpoints, or personal browsing data.

## Pull requests

Describe the user-visible problem, the resulting behavior, and how you verified it. Include screenshots for visible UI changes and manual steps for checks you could not run. Keep commit messages specific and PRs small enough to review.

## Report a bug

Use [GitHub Issues](https://github.com/Glimor/proxity/issues) and include your Windows build (`winver`), Proxity version from Settings, reproduction steps, and relevant logs from `%ProgramData%\Proxity\logs`. Redact usernames, credentials, private endpoints, and sensitive paths.

Report security vulnerabilities privately using [SECURITY.md](SECURITY.md).

## Branches, triage, and releases

Use `feat/`, `fix/`, `docs/`, `chore/`, or `deps/` plus a short descriptive name.
Open a focused PR into main and use the supplied PR checklist. Use the issue
forms; maintainers assign priority labels after triage. General support belongs
in Discussions. See [SUPPORT.md](SUPPORT.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md),
and [release policy](docs/releasing.md). This repository can also be reused via
GitHub's template feature; follow [the template guide](docs/template-guide.md).
