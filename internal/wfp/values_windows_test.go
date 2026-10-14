//go:build windows

package wfp

import (
	"testing"
	"unsafe"
)

func TestFWPByteBlobDataType(t *testing.T) {
	if cFWP_BYTE_BLOB_TYPE != 12 {
		t.Fatalf("FWP_BYTE_BLOB_TYPE = %d, want 12", cFWP_BYTE_BLOB_TYPE)
	}
}

func TestUint64ValueUsesKeptPointer(t *testing.T) {
	const want = uint64(4000)
	var keep []any

	value := uint64Value(want, &keep)
	if value._type != cFWP_UINT64 {
		t.Fatalf("type = %v, want %v", value._type, cFWP_UINT64)
	}
	if len(keep) != 1 {
		t.Fatalf("keep length = %d, want 1", len(keep))
	}
	p, ok := keep[0].(*uint64)
	if !ok {
		t.Fatalf("kept value type = %T, want *uint64", keep[0])
	}
	if *p != want {
		t.Fatalf("pointed value = %d, want %d", *p, want)
	}
	if value.value != uintptr(unsafe.Pointer(p)) {
		t.Fatalf("stored pointer = %#x, want %#x", value.value, uintptr(unsafe.Pointer(p)))
	}
}
