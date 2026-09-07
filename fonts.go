package kyfram

import _ "embed"

//go:embed examples/assets/fonts/ArchivoBlack-Regular.ttf
var displayFont []byte

// DisplayFont returns the bundled display typeface: Archivo Black,
// used for video titles (via --font or the font prop) and the preview
// UI's own headings. OFL licensed, see examples/assets/fonts/.
func DisplayFont() []byte { return displayFont }
