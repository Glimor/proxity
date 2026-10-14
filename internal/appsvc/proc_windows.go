//go:build windows

package appsvc

import (
	"context"
	"unsafe"

	"github.com/Glimor/proxity/internal/domain"
	"golang.org/x/sys/windows"
)

// ListRunningProcesses enumerates running processes with their exe paths
// (Toolhelp snapshot + QueryFullProcessImageName; protected/system processes
// may come back without a path — the GUI can still show the name).
func (s *Service) ListRunningProcesses(ctx context.Context) ([]ProcessInfo, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(snap)

	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []ProcessInfo
	if err := windows.Process32First(snap, &e); err != nil {
		return out, nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		name := windows.UTF16ToString(e.ExeFile[:])
		pi := ProcessInfo{PID: int(e.ProcessID), Name: name}
		if p := processImagePath(e.ProcessID); p != "" {
			if c, err := domain.CanonicalPath(p); err == nil {
				p = c
			}
			pi.Path = p
		}
		out = append(out, pi)
		if err := windows.Process32Next(snap, &e); err != nil {
			break
		}
	}
	return out, nil
}

func processImagePath(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 32768)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}
