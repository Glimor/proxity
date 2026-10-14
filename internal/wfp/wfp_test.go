//go:build windows

package wfp

import (
	"os"
	"strings"
	"testing"
	"unsafe"
)

// Layout sizes must match fwpmtypes.h exactly on 64-bit — wrong sizes
// corrupt the BFE marshaling buffer.
func TestStructSizes(t *testing.T) {
	cases := []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"wtFwpByteBlob", unsafe.Sizeof(wtFwpByteBlob{}), wtFwpByteBlob_Size},
		{"wtFwpConditionValue0", unsafe.Sizeof(wtFwpConditionValue0{}), wtFwpConditionValue0_Size},
		{"wtFwpmFilter0", unsafe.Sizeof(wtFwpmFilter0{}), wtFwpmFilter0_Size},
		{"wtFwpmFilterCondition0", unsafe.Sizeof(wtFwpmFilterCondition0{}), wtFwpmFilterCondition0_Size},
		{"wtFwpmSession0", unsafe.Sizeof(wtFwpmSession0{}), wtFwpmSession0_Size},
		{"wtFwpmSublayer0", unsafe.Sizeof(wtFwpmSublayer0{}), wtFwpmSublayer0_Size},
		{"wtFwpmProvider0", unsafe.Sizeof(wtFwpmProvider0{}), wtFwpProvider0_Size},
		{"wtFwpValue0", unsafe.Sizeof(wtFwpValue0{}), wtFwpValue0_Size},
		{"wtFwpV4AddrAndMask", unsafe.Sizeof(wtFwpV4AddrAndMask{}), wtFwpV4AddrAndMask_Size},
		{"wtFwpV6AddrAndMask", unsafe.Sizeof(wtFwpV6AddrAndMask{}), wtFwpV6AddrAndMask_Size},
		{"wtFwpRange0", unsafe.Sizeof(wtFwpRange0{}), wtFwpRange0_Size},
		{"wtFwpmDisplayData0", unsafe.Sizeof(wtFwpmDisplayData0{}), wtFwpmDisplayData0_Size},
		{"wtFwpmAction0", unsafe.Sizeof(wtFwpmAction0{}), wtFwpmAction0_Size},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: size %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestToDevicePath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path")
	}
	dev, err := ToDevicePath(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dev, `\Device\`) {
		t.Fatalf("expected device path, got %q", dev)
	}
	if strings.Contains(dev, ":") {
		t.Fatalf("device path still has a drive letter: %q", dev)
	}
}

func TestAppIDFromPath(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Skip("no executable path")
	}
	blob, err := AppIDFromPath(exe)
	if err != nil {
		t.Skipf("FwpmGetAppIdFromFileName0 unavailable: %v", err)
	}
	if len(blob) < 4 || len(blob)%2 != 0 {
		t.Fatalf("unexpected app-id blob length %d", len(blob))
	}
}

func TestPrefixMask4(t *testing.T) {
	for in, want := range map[int]uint32{
		0: 0, 8: 0xff000000, 12: 0xfff00000, 16: 0xffff0000,
		24: 0xffffff00, 32: 0xffffffff,
	} {
		if got := prefixMask4(in); got != want {
			t.Errorf("prefixMask4(%d)=%#x want %#x", in, got, want)
		}
	}
}
