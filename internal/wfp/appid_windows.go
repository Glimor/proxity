//go:build windows

package wfp

import (
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// AppIDFromPath converts a Win32 path (C:\Dir\App.exe) to the WFP application
// ID blob used by the FWPM_CONDITION_ALE_APP_ID condition. The path is first
// resolved to device form (\Device\HarddiskVolumeN\Dir\App.exe) via
// QueryDosDeviceW, then FwpmGetAppIdFromFileName0 produces the blob; if the
// device conversion fails the plain path is tried as a fallback.
// The target file must exist.
func AppIDFromPath(exePath string) ([]byte, error) {
	p := exePath
	if dev, err := ToDevicePath(exePath); err == nil {
		p = dev
	}
	ptr, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return nil, err
	}
	var blob *wtFwpByteBlob
	if err := fwpmGetAppIdFromFileName0(ptr, unsafe.Pointer(&blob)); err != nil {
		if p != exePath {
			// Retry with the unresolved Win32 path — fwpuclnt accepts DOS
			// paths on current Windows versions.
			if ptr, err = windows.UTF16PtrFromString(exePath); err != nil {
				return nil, err
			}
			if err := fwpmGetAppIdFromFileName0(ptr, unsafe.Pointer(&blob)); err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}
	}
	defer fwpmFreeMemory0(unsafe.Pointer(&blob))
	out := make([]byte, blob.size)
	copy(out, unsafe.Slice(blob.data, blob.size))
	return out, nil
}

// AppIDToPath renders a WFP app-id blob (UTF-16LE device path, e.g.
// "\device\harddiskvolume3\dir\app.exe") as a Win32 path by reversing
// QueryDosDeviceW across the drive letters. Returns "" when the blob is
// undecodable or the volume isn't a DOS drive (e.g. a system-internal path).
func AppIDToPath(blob []byte) string {
	if len(blob) == 0 || len(blob)%2 != 0 {
		return ""
	}
	u16 := unsafe.Slice((*uint16)(unsafe.Pointer(&blob[0])), len(blob)/2)
	dev := strings.ToLower(windows.UTF16ToString(u16))

	// Map each DOS drive letter to its device prefix once per call — drives
	// are few and this only runs for classify-drop events.
	for c := 'A'; c <= 'Z'; c++ {
		vol := string(c) + ":"
		volPtr, err := windows.UTF16PtrFromString(vol)
		if err != nil {
			continue
		}
		var buf [256]uint16
		n, err := queryDosDeviceW(volPtr, &buf[0], uint32(len(buf)))
		if err != nil || n == 0 {
			continue
		}
		target := strings.TrimRight(windows.UTF16ToString(buf[:n]), "\x00")
		for _, t := range strings.Split(target, "\x00") {
			if t == "" {
				continue
			}
			if strings.HasPrefix(dev, strings.ToLower(t)+"\\") {
				return vol + dev[len(t):]
			}
		}
	}
	return dev // best effort: return the device path
}

// ToDevicePath converts "C:\Dir\File.exe" to "\Device\HarddiskVolumeN\Dir\File.exe".
func ToDevicePath(exePath string) (string, error) {
	vol := filepath.VolumeName(exePath)
	if vol == "" || !strings.HasSuffix(vol, ":") || strings.HasPrefix(exePath, `\\`) {
		return "", windows.ERROR_INVALID_NAME
	}
	volPtr, err := windows.UTF16PtrFromString(vol)
	if err != nil {
		return "", err
	}
	buf := make([]uint16, 256)
	n, err := queryDosDeviceW(volPtr, &buf[0], uint32(len(buf)))
	if err != nil {
		return "", err
	}
	// The buffer holds one or more NUL-terminated targets (e.g. for
	// multi-target volumes); the first is the device path.
	target := windows.UTF16ToString(buf[:n])
	target = strings.TrimRight(target, "\x00")
	return target + exePath[len(vol):], nil
}
