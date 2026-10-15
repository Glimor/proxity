//go:build windows

package engine

import "github.com/Glimor/proxity/internal/windivert"

// SelfCheck verifies WinDivert.dll can be located and loaded. Opening a
// capture handle (admin check) happens in Start.
func SelfCheck() error { return windivert.Available() }
