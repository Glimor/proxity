# Dependency security status

Reviewed 2026-10-15. This is a dated assessment, not a permanent exemption.

## braces / GHSA-vfj7-8cjw-p6xm

`braces` 3.0.3 is pulled into the development toolchain by Tailwind CSS 3 through
chokidar/micromatch/fast-glob. The advisory reports stack exhaustion on deeply
nested patterns and currently lists no patched braces release. npm audit counts
five high findings along this same dependency chain.

The normal build uses repository-controlled Tailwind content patterns. The
production frontend bundle does not run this build-time glob parser, and
`npm audit --omit=dev` reports no findings at the time of review. Do not pass
untrusted glob patterns into the build toolchain. Fork workflows require
maintainer approval and receive no write token or repository secrets by default.

Keep the Dependabot alert open until fixed. Track a separate Tailwind 4 migration
with visual regression verification, or adopt a patched dependency chain when
available. Do not force an unrelated major CSS migration into a maintenance PR.

## Echo / GHSA-vfp3-v2gw-7wfq

The manifest already selects github.com/labstack/echo/v4 4.15.3, the patched
version listed by the advisory. Verify the dependency graph and reachable-code
scan before closing any stale alert. Wails uses Echo for its development server;
this does not excuse keeping an unpatched version.

## Review process

Dependabot checks Go, npm, and Actions weekly. CI audits production npm
packages and runs govulncheck; CodeQL runs on pushes, PRs, and weekly. Review
all dependency alerts, including development-only findings, and update this
assessment when versions or exposure change.
