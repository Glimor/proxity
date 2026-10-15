package secrets

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

type Cipher struct {
	machineScope bool
}

// NewCipher returns a DPAPI cipher. machineScope=true uses
// CRYPTPROTECT_LOCAL_MACHINE so any local principal (incl. the SYSTEM service)
// can unprotect; false scopes to the current user.
func NewCipher(machineScope bool) *Cipher {
	return &Cipher{machineScope: machineScope}
}

func (c *Cipher) flags() uint32 {
	f := uint32(windows.CRYPTPROTECT_UI_FORBIDDEN)
	if c.machineScope {
		f |= windows.CRYPTPROTECT_LOCAL_MACHINE
	}
	return f
}

func (c *Cipher) Protect(plaintext []byte) ([]byte, error) {
	var in, out windows.DataBlob
	if len(plaintext) > 0 {
		in.Data = &plaintext[0]
		in.Size = uint32(len(plaintext))
	}
	if err := windows.CryptProtectData(&in, nil, nil, 0, nil, c.flags(), &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	buf := make([]byte, out.Size)
	copy(buf, unsafe.Slice(out.Data, out.Size))
	return buf, nil
}

func (c *Cipher) Unprotect(ciphertext []byte) ([]byte, error) {
	var in, out windows.DataBlob
	if len(ciphertext) > 0 {
		in.Data = &ciphertext[0]
		in.Size = uint32(len(ciphertext))
	}
	if err := windows.CryptUnprotectData(&in, nil, nil, 0, nil, c.flags(), &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	buf := make([]byte, out.Size)
	copy(buf, unsafe.Slice(out.Data, out.Size))
	return buf, nil
}
