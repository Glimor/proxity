//go:build windows

// Package guiicon extracts a 32x32 PNG (as a base64 data-URL) from a Windows
// executable using shell32 SHGetFileInfoW + GDI, no CGO. When extraction
// fails a generated generic icon is returned instead, so callers always get
// something renderable.
package guiicon

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/png"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32 = windows.NewLazySystemDLL("shell32.dll")
	user32  = windows.NewLazySystemDLL("user32.dll")
	gdi32   = windows.NewLazySystemDLL("gdi32.dll")

	procSHGetFileInfoW = shell32.NewProc("SHGetFileInfoW")
	procDrawIconEx     = user32.NewProc("DrawIconEx")
	procDestroyIcon    = user32.NewProc("DestroyIcon")
	procGetIconInfo    = user32.NewProc("GetIconInfo")

	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
)

const (
	shgfiIcon              = 0x000000100
	shgfiLargeIcon         = 0x000000000
	shgfiUseFileAttributes = 0x000000010
	fileAttributeNormal    = 0x00000080

	diNormal = 0x0003
	dibRGB   = 0
	biRGB    = 0
	iconSize = 32
)

type shFileInfoW struct {
	hIcon         windows.Handle
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [windows.MAX_PATH]uint16
	szTypeName    [80]uint16
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  windows.Handle
	hbmColor windows.Handle
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

type bitmapInfo struct {
	bmiHeader bitmapInfoHeader
	bmiColors [1]uint32
}

// DataURL returns a data:image/png;base64 URL for the exe at path. On any
// failure it returns the generated fallback icon, never "".
func DataURL(path string) string {
	img, err := extract(path)
	if err != nil || img == nil {
		return Fallback()
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Fallback()
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// PNG returns the encoded PNG bytes for the exe at path (testable helper).
func PNG(path string) ([]byte, error) {
	img, err := extract(path)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func extract(path string) (*image.RGBA, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	var shfi shFileInfoW
	flags := uintptr(shgfiIcon | shgfiLargeIcon | shgfiUseFileAttributes)
	r, _, _ := procSHGetFileInfoW.Call(
		uintptr(unsafe.Pointer(p)),
		fileAttributeNormal,
		uintptr(unsafe.Pointer(&shfi)),
		uintptr(unsafe.Sizeof(shfi)),
		flags,
	)
	if r == 0 || shfi.hIcon == 0 {
		return nil, windows.ERROR_FILE_NOT_FOUND
	}
	defer procDestroyIcon.Call(uintptr(shfi.hIcon))
	return renderIcon(shfi.hIcon)
}

// renderIcon draws the icon into a 32bpp top-down DIB and converts BGRA→RGBA.
func renderIcon(hIcon windows.Handle) (*image.RGBA, error) {
	var ii iconInfo
	r, _, _ := procGetIconInfo.Call(uintptr(hIcon), uintptr(unsafe.Pointer(&ii)))
	if r == 0 {
		return nil, windows.ERROR_INVALID_HANDLE
	}
	if ii.hbmColor != 0 {
		defer procDeleteObject.Call(uintptr(ii.hbmColor))
	}
	if ii.hbmMask != 0 {
		defer procDeleteObject.Call(uintptr(ii.hbmMask))
	}

	hdc, _, _ := procCreateCompatibleDC.Call(0)
	if hdc == 0 {
		return nil, windows.ERROR_NOT_ENOUGH_MEMORY
	}
	defer procDeleteDC.Call(hdc)

	bi := bitmapInfo{
		bmiHeader: bitmapInfoHeader{
			biSize:        uint32(unsafe.Sizeof(bitmapInfoHeader{})),
			biWidth:       iconSize,
			biHeight:      -iconSize, // top-down
			biPlanes:      1,
			biBitCount:    32,
			biCompression: biRGB,
		},
	}
	var bits unsafe.Pointer
	hbm, _, _ := procCreateDIBSection.Call(
		hdc,
		uintptr(unsafe.Pointer(&bi)),
		dibRGB,
		uintptr(unsafe.Pointer(&bits)),
		0, 0,
	)
	if hbm == 0 || bits == nil {
		return nil, windows.ERROR_NOT_ENOUGH_MEMORY
	}
	defer procDeleteObject.Call(hbm)
	old, _, _ := procSelectObject.Call(hdc, hbm)
	defer procSelectObject.Call(hdc, old)

	if r, _, _ := procDrawIconEx.Call(hdc, 0, 0, uintptr(hIcon), iconSize, iconSize, 0, 0, diNormal); r == 0 {
		return nil, windows.ERROR_INVALID_PARAMETER
	}

	const n = iconSize * iconSize
	src := unsafe.Slice((*byte)(bits), n*4)
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	for i := 0; i < n; i++ {
		b, g, rr, a := src[i*4], src[i*4+1], src[i*4+2], src[i*4+3]
		if a == 0 {
			// Pixels under a mask-only icon come back a=0 on black; keep the
			// transparency — the UI renders it on its own background.
			continue
		}
		img.Pix[i*4+0] = rr
		img.Pix[i*4+1] = g
		img.Pix[i*4+2] = b
		img.Pix[i*4+3] = a
	}
	return img, nil
}
