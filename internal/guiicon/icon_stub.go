//go:build !windows

package guiicon

// DataURL falls back on non-Windows platforms; the product targets Windows.
func DataURL(_ string) string { return Fallback() }

// PNG is unsupported off Windows.
func PNG(_ string) ([]byte, error) { return FallbackPNG(), nil }
