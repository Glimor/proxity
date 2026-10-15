package guiicon

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"sync"
)

var (
	fallbackOnce sync.Once
	fallbackURL  string
)

// Fallback returns a generated generic-application icon as a data URL. It is
// used whenever a real icon cannot be extracted.
func Fallback() string {
	fallbackOnce.Do(func() {
		fallbackURL = "data:image/png;base64," + base64.StdEncoding.EncodeToString(FallbackPNG())
	})
	return fallbackURL
}

// FallbackPNG renders a simple slate rounded-square with a white glyph, used
// both as the per-exe fallback and (larger) as the app/tray icon.
func FallbackPNG() []byte {
	const s = 64
	img := image.NewRGBA(image.Rect(0, 0, s, s))
	bg := color.RGBA{R: 0x4f, G: 0x46, B: 0xe5, A: 0xff} // indigo-600
	fg := color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff} // white

	// Rounded-rect background (approximation: two rects + corner circles).
	draw.Draw(img, image.Rect(4, 0, s-4, s), &image.Uniform{bg}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, 4, s, s-4), &image.Uniform{bg}, image.Point{}, draw.Src)
	for cy := 0; cy <= 1; cy++ {
		for cx := 0; cx <= 1; cx++ {
			cx0, cy0 := 4+cx*(s-8), 4+cy*(s-8)
			for dy := -4; dy <= 4; dy++ {
				for dx := -4; dx <= 4; dx++ {
					if dx*dx+dy*dy <= 16 {
						img.Set(cx0+dx, cy0+dy, bg)
					}
				}
			}
		}
	}
	// "P" glyph: stem + bowl, blocky.
	draw.Draw(img, image.Rect(22, 16, 30, 48), &image.Uniform{fg}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(22, 16, 42, 24), &image.Uniform{fg}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(22, 30, 42, 38), &image.Uniform{fg}, image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(34, 24, 42, 30), &image.Uniform{fg}, image.Point{}, draw.Src)

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
