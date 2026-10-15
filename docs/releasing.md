# Releases

Use short-lived `feat/`, `fix/`, `docs/`, `chore/`, or `deps/` branches and a PR
into `main`. Squash merging, signed merge commits, passing required checks,
resolved review conversations, and an up-to-date branch are required. With one
maintainer there is no mandatory self-approval; add one required review when a
second active maintainer is available. Release branches are optional and share
the protected-branch policy. Tags are immutable.

1. Update CHANGELOG.md, docs/release-notes.md, and version metadata as needed.
2. Merge the PR after CI and CodeQL pass, then wait for their main push runs.
3. Before a stable release, verify installation, upgrade, reboot/autostart,
   service stop/start, kill switches, DNS restoration and uninstall on an
   isolated Windows machine. Record OS/build and results without private data.
4. Create a new `vX.Y.Z` tag at the current main commit; use `vX.Y.Z-rc.N` for
   evaluation builds. Never move or reuse a published tag.
5. The release workflow verifies the tag/main/checks, builds on Windows, includes
   third-party notices and inventory, stages a draft, and publishes after all
   assets are uploaded. Prerelease suffixes are marked as prereleases.
6. Verify the installer/ZIP/checksums and release notes. A failed upload may
   leave a draft; inspect it before retrying. Published releases are immutable.

The workflow intentionally does not install or execute the service on a shared
CI runner. Signing requires a publisher certificate and a separately designed
signing process; current binaries remain unsigned and are documented as such.
