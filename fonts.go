package kyfram

import (
	_ "embed"
	"sync"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

//go:embed examples/assets/fonts/ArchivoBlack-Regular.ttf
var displayFont []byte

// DisplayFont returns the bundled display typeface: Archivo Black,
// used for video titles (via --font or the font prop) and the preview
// UI's own headings. OFL licensed, see examples/assets/fonts/.
func DisplayFont() []byte { return displayFont }

var (
	bundledFaceOnce sync.Once
	bundledFaceFont *truetype.Font
)

// bundledFace builds a text face from the embedded bytes so installs work
// outside the repo (the repo-relative default path only exists in dev).
func bundledFace(size float64) (font.Face, bool) {
	bundledFaceOnce.Do(func() {
		bundledFaceFont, _ = truetype.Parse(displayFont)
	})
	if bundledFaceFont == nil {
		return nil, false
	}
	return truetype.NewFace(bundledFaceFont, &truetype.Options{
		Size: size,
		DPI:  72,
	}), true
}

// setFace loads an explicit TTF path, or the embedded bundle when the
// resolved path is the repo-relative default. Reports whether text can draw.
func setFace(dc *gg.Context, path string, size float64) bool {
	if path == defaultFont {
		face, ok := bundledFace(size)
		if !ok {
			return false
		}
		dc.SetFontFace(face)
		return true
	}
	return dc.LoadFontFace(path, size) == nil
}
