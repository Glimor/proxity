# Third-party notices

Proxity is distributed under GPL-3.0; see LICENSE. Third-party components retain
their own licenses and copyright notices. This file is a component guide, not a
replacement for their license texts.

## WinDivert

The bundled WinDivert 2.2.x DLL, signed driver and header are from
https://github.com/basil00/WinDivert. The bundled license offers LGPL-3.0 or
GPL-2.0; Proxity uses the LGPL-3.0 option. The full bundled license is at
third_party/windivert/LICENSE. Obtain the corresponding upstream source for the
bundled version from https://github.com/basil00/WinDivert/releases/tag/v2.2.2.
Keep its license and source availability information with redistributed binaries.

## WireGuard for Windows

Selected WFP definitions and bindings are adapted from WireGuard for Windows,
copyright WireGuard LLC and contributors, under the MIT license. Original
NOTICE and SPDX headers remain in the affected source files. The full MIT
license is at third_party/wireguard/LICENSE.

## Go and frontend dependencies

Exact Go dependencies are recorded in go.mod and go.sum; frontend dependencies
are recorded in frontend/package-lock.json. Their upstream source packages
contain their respective license texts. Release bundles include a dependency
inventory and an archive of dependency license files, generated from the exact
installed dependencies during the build.
