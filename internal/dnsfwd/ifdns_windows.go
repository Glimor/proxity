//go:build windows

package dnsfwd

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// This file points the active physical interfaces' DNS at the loopback
// forwarder and restores the previous config afterwards.
//
// Primary path: SetInterfaceDnsSettings / GetInterfaceDnsSettings
// (iphlpapi.dll, Windows 10 1903+) via
// ConvertInterfaceIndexToLuid + ConvertInterfaceLuidToGuid to reach the
// adapter GUID Go's net package doesn't expose. Fallback path (pre-1903):
// netsh interface ip(v4|v6) set dnsservers.

var (
	modiphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")

	procConvertInterfaceIndexToLuid = modiphlpapi.NewProc("ConvertInterfaceIndexToLuid")
	procConvertInterfaceLuidToGuid  = modiphlpapi.NewProc("ConvertInterfaceLuidToGuid")
	procSetInterfaceDnsSettings     = modiphlpapi.NewProc("SetInterfaceDnsSettings")
	procGetInterfaceDnsSettings     = modiphlpapi.NewProc("GetInterfaceDnsSettings")
	procFreeInterfaceDnsSettings    = modiphlpapi.NewProc("FreeInterfaceDnsSettings")
)

// DNS_INTERFACE_SETTINGS version 1 (netioapi.h).
const (
	cDnsSettingsVersion1  = 1
	cDnsSettingIPv6       = 0x0001
	cDnsSettingNameserver = 0x0002
)

// wtDnsInterfaceSettings mirrors DNS_INTERFACE_SETTINGS v1 (x64).
type wtDnsInterfaceSettings struct {
	version             uint32
	_                   uint32
	flags               uint64
	domain              *uint16
	nameServer          *uint16
	searchList          *uint16
	registrationEnabled uint32
	registerAdapterName uint32
	enableLLMNR         uint32
	queryAdapterName    uint32
	profileNameServer   *uint16
}

func ifaceGUID(index int) (windows.GUID, error) {
	var luid uint64
	r1, _, e1 := syscall.Syscall(procConvertInterfaceIndexToLuid.Addr(), 2,
		uintptr(index), uintptr(unsafe.Pointer(&luid)), 0)
	if r1 != 0 {
		return windows.GUID{}, errnoFrom(e1)
	}
	var g windows.GUID
	r1, _, e1 = syscall.Syscall(procConvertInterfaceLuidToGuid.Addr(), 2,
		uintptr(unsafe.Pointer(&luid)), uintptr(unsafe.Pointer(&g)), 0)
	if r1 != 0 {
		return windows.GUID{}, errnoFrom(e1)
	}
	return g, nil
}

// dnsAPI reports whether the Win10 1903+ per-interface DNS API exists.
func dnsAPI() bool {
	return procSetInterfaceDnsSettings.Find() == nil &&
		procGetInterfaceDnsSettings.Find() == nil
}

// getDNSServers returns the comma-separated name-server list configured on
// the interface ("" = DHCP / none).
func getDNSServers(g windows.GUID, v6 bool) (string, error) {
	st := wtDnsInterfaceSettings{version: cDnsSettingsVersion1}
	if v6 {
		st.flags = cDnsSettingIPv6
	}
	r1, _, e1 := syscall.Syscall(procGetInterfaceDnsSettings.Addr(), 2,
		uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&st)), 0)
	if r1 != 0 {
		return "", errnoFrom(e1)
	}
	defer syscall.Syscall(procFreeInterfaceDnsSettings.Addr(), 1,
		uintptr(unsafe.Pointer(&st)), 0, 0)
	if st.nameServer == nil {
		return "", nil
	}
	return windows.UTF16PtrToString(st.nameServer), nil
}

// setDNSServers configures the interface's DNS servers ("a,b,c") via the
// modern API; servers="" restores DHCP-assigned DNS.
func setDNSServers(g windows.GUID, v6 bool, servers string) error {
	st := wtDnsInterfaceSettings{version: cDnsSettingsVersion1}
	st.flags = cDnsSettingNameserver
	if v6 {
		st.flags |= cDnsSettingIPv6
	}
	var ns *uint16
	if servers != "" {
		p, err := windows.UTF16PtrFromString(servers)
		if err != nil {
			return err
		}
		ns = p
	}
	st.nameServer = ns
	r1, _, e1 := syscall.Syscall(procSetInterfaceDnsSettings.Addr(), 2,
		uintptr(unsafe.Pointer(&g)), uintptr(unsafe.Pointer(&st)), 0)
	if r1 != 0 {
		return errnoFrom(e1)
	}
	return nil
}

// netshSetDNS is the legacy fallback. servers="" means DHCP.
func netshSetDNS(ifaceName string, v6 bool, servers string) error {
	fam := "ipv4"
	if v6 {
		fam = "ipv6"
	}
	var args []string
	if servers == "" {
		args = []string{"interface", fam, "set", "dnsservers", ifaceName, "dhcp"}
	} else {
		args = []string{"interface", fam, "set", "dnsservers", ifaceName, "static", servers}
	}
	out, err := exec.Command("netsh", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("netsh %s: %v: %s", fam, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// IfaceBackup remembers one interface's previous DNS configuration.
type IfaceBackup struct {
	Index     int    `json:"index"`
	Name      string `json:"name"`
	GUID      string `json:"guid"`
	V4Servers string `json:"v4_servers"` // "" = DHCP
	V6Servers string `json:"v6_servers"`
}

// DNSBackup is the persisted pre-secure-DNS adapter state (settings key
// "dns_backup").
type DNSBackup struct {
	Interfaces []IfaceBackup `json:"interfaces"`
}

// activeIfaces lists up, non-loopback interfaces with a hardware address —
// the physical interfaces whose DNS we take over.
func activeIfaces() ([]net.Interface, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []net.Interface
	for _, i := range ifs {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		if i.Flags&net.FlagPointToPoint != 0 || i.Flags&net.FlagMulticast == 0 {
			continue // tunnels / pseudo interfaces
		}
		out = append(out, i)
	}
	return out, nil
}

// CaptureDNSBackup records every active interface's current DNS servers.
// Individual-interface failures are tolerated (skipped ifaces just won't be
// restored).
func CaptureDNSBackup(log func(string, error)) (*DNSBackup, error) {
	ifs, err := activeIfaces()
	if err != nil {
		return nil, err
	}
	b := &DNSBackup{}
	for _, i := range ifs {
		entry := IfaceBackup{Index: i.Index, Name: i.Name}
		g, err := ifaceGUID(i.Index)
		if err != nil {
			if log != nil {
				log(i.Name, err)
			}
			continue
		}
		entry.GUID = g.String()
		if dnsAPI() {
			if v4, err := getDNSServers(g, false); err == nil {
				entry.V4Servers = v4
			}
			if v6, err := getDNSServers(g, true); err == nil {
				entry.V6Servers = v6
			}
		}
		b.Interfaces = append(b.Interfaces, entry)
	}
	return b, nil
}

// SetLoopbackDNS points every active physical interface at the loopback
// forwarder (127.0.0.1 / ::1). Errors per-interface are aggregated but
// non-fatal — the WFP port-53 block still prevents plaintext leaks.
func SetLoopbackDNS() []error {
	var errs []error
	ifs, err := activeIfaces()
	if err != nil {
		return []error{err}
	}
	modern := dnsAPI()
	for _, i := range ifs {
		if modern {
			g, err := ifaceGUID(i.Index)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", i.Name, err))
				continue
			}
			if err := setDNSServers(g, false, "127.0.0.1"); err != nil {
				errs = append(errs, fmt.Errorf("%s v4: %w", i.Name, err))
			}
			if err := setDNSServers(g, true, "::1"); err != nil {
				errs = append(errs, fmt.Errorf("%s v6: %w", i.Name, err))
			}
			continue
		}
		if err := netshSetDNS(i.Name, false, "127.0.0.1"); err != nil {
			errs = append(errs, err)
		}
		if err := netshSetDNS(i.Name, true, "::1"); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// RestoreDNS puts every recorded interface's DNS back (static servers or
// DHCP). Missing/failed interfaces are reported but don't stop the rest.
func RestoreDNS(b *DNSBackup) []error {
	if b == nil {
		return nil
	}
	var errs []error
	modern := dnsAPI()
	for _, ib := range b.Interfaces {
		g, gerr := parseGUID(ib.GUID)
		restores := []struct {
			v6      bool
			servers string
		}{
			{false, ib.V4Servers},
			{true, ib.V6Servers},
		}
		for _, r := range restores {
			var err error
			switch {
			case modern && gerr == nil && r.servers != "":
				err = setDNSServers(g, r.v6, r.servers)
			default:
				// "" (was DHCP) or no modern API → netsh.
				err = netshSetDNS(ib.Name, r.v6, r.servers)
			}
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", ib.Name, err))
			}
		}
	}
	return errs
}

func parseGUID(s string) (windows.GUID, error) {
	return windows.GUIDFromString(s)
}

func errnoFrom(e error) error {
	if errno, ok := e.(syscall.Errno); ok && errno != 0 {
		return errno
	}
	if e == syscall.Errno(0) || e == nil {
		return errors.New("unknown error")
	}
	return e
}
