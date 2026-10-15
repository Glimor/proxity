## Installation and verification

Download `Proxity-amd64-installer.exe` for normal setup. The ZIP contains the GUI,
service, WinDivert components and license notices; it is not a fully portable mode.
Compare the download with `checksums.txt` using `Get-FileHash -Algorithm SHA256`.
The source for this build is available in the source archives attached to this tag.

Windows 10/11 x64 and administrator consent are required for service/driver setup.
Binaries are currently unsigned; checksums do not replace publisher signing.
Back up profiles before uninstalling; uninstall removes service data.

CI covers Go unit tests, frontend typecheck/build, mocked GUI tests, Windows
compilation, reachable Go vulnerabilities, and CodeQL. It does not replace
privileged WFP/WinDivert, real-network leak, installer/reboot, or DNS-restoration
checks on a dedicated Windows machine. Tags with a suffix such as `-rc.1` are
prereleases intended for evaluation, not a claim of production readiness.

See SECURITY.md for the security model and docs/security-status.md for known
dependency findings. Never attach live credentials or proxy exports to issues.
