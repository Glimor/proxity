// Package version exposes the build version of the Proxity binaries.
//
// Release builds stamp it via
//
//	-ldflags "-X github.com/Glimor/proxity/internal/version.Version=X.Y.Z"
//
// (see build.ps1 and .github/workflows). "dev" marks unversioned local
// builds. The GUI and the service each carry their own stamp; the IPC ping
// response includes the service's version so the GUI can warn on mismatch
// (e.g. installer upgraded one binary but not the other).
package version

// Version is the semver stamp for this binary.
var Version = "dev"
