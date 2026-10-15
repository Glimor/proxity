# Reusing this repository as a template

Use **Use this template > Create a new repository** on GitHub. This copies the
source and versioned .github files with a new history. It does **not** clone
repository settings, rulesets, labels, security toggles, Discussions categories,
secrets, environments, release history, or installed integrations.

## Adapt the copied project before accepting contributions

- Replace Proxity branding, module path, package names, output names and Windows
  service/IPC/WFP identifiers if creating another application. Preserve license
  and third-party attribution obligations.
- Replace `Glimor/proxity` links and `@Glimor` in CODEOWNERS, community documents
  and issue forms. Select your own owner and private conduct/security contacts.
- Adapt CI, CodeQL languages, build/release scripts and required check names to
  the new technology stack. The supplied implementation targets Windows/Go/Vue.
- Review `.github/repository/*.json`. The default policy is for a solo maintainer;
  use a required approving review when there are multiple active maintainers.
- Start with your own changelog and version; do not copy Proxity release claims.

## Apply the settings to the new public repository

Install GitHub CLI, run `gh auth login` with administration rights for the target,
and use PowerShell 7:

```powershell
# Read-only plan first; specify the exact target repository.
./scripts/setup-github.ps1 -Repository OWNER/REPOSITORY
# After reviewing the plan and versioned JSON:
./scripts/setup-github.ps1 -Repository OWNER/REPOSITORY -Apply
```

The script refuses private repositories and never changes visibility. It
updates matching labels and named rulesets rather than creating duplicates;
other labels/rulesets are preserved. It sets squash-only merging, head-branch
cleanup, read-only workflow defaults, full-SHA action pinning, approval for all
external contributors, dependency alerts/security fixes, private vulnerability
reporting, immutable releases, secret scanning and push protection. The allowlist permits GitHub-owned
actions plus softprops/action-gh-release. Adapt it for different workflows.

Required checks are bound to GitHub Actions. CI must successfully run on the
first PR before merging. Workflow files provide advanced CodeQL analysis;
do not enable default CodeQL setup at the same time. Verify CodeQL results and
Security settings after the first run. Also configure About/topics and Discussions
categories (Announcements, Q&A, Ideas) in GitHub;
these project-specific/UI-only settings are not copied by the template script.

Before publishing, scan source/history and screenshots for secrets and personal
data. Use a public GitHub noreply identity for commits. Never import an old
repository's .git directory or private environment files into the new project.
