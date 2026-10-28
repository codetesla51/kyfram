package kyfram

import (
	"image"
	"image/draw"
	"math"
)

// hasEffects reports whether an entity asks for a per-entity effect.
// Effects are rendered through a temporary layer so shadows and glows
// can use the entity's own alpha instead of its rectangular bounds.
func hasEffects(props map[string]any) bool {
	return propStr(props, "shadowColor", "") != "" ||
		propStr(props, "glow", "") != "" ||
		propNum(props, "blur", 0) > 0
}

// applyEffects builds a new layer with shadow, glow, and blur applied in
// that order. Shadow/glow are silhouettes of the source alpha, so they
// follow text and transparent PNG edges rather than a bounding box.
func applyEffects(src *image.RGBA, props map[string]any) *image.RGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))

	if col := propStr(props, "shadowColor", ""); col != "" {
		if c, err := parseColor(col); err == nil {
			r := int(math.Round(propNum(props, "shadowBlur", 10)))
			if r < 0 {
				r = 0
			}
			a := blurPlane(extractAlpha(src), w, h, r)
			shadow := colorizeAlpha(a, w, h, c, propNum(props, "shadowAlpha", 1))
			dx := int(math.Round(propNum(props, "shadowX", 0)))
			dy := int(math.Round(propNum(props, "shadowY", 0)))
			draw.Draw(out, out.Bounds(), shadow, image.Pt(-dx, -dy), draw.Over)
		}
	}

	if col := propStr(props, "glow", ""); col != "" {
		if c, err := parseColor(col); err == nil {
			r := int(math.Round(propNum(props, "glowBlur", 16)))
			if r < 0 {
				r = 0
			}
			a := blurPlane(extractAlpha(src), w, h, r)
			glow := colorizeAlpha(a, w, h, c, propNum(props, "glowAlpha", 1))
			draw.Draw(out, out.Bounds(), glow, image.Point{}, draw.Over)
		}
	}

	base := src
	if r := int(math.Round(propNum(props, "blur", 0))); r > 0 {
		base = blurRGBA(src, r)
	}
	draw.Draw(out, out.Bounds(), base, image.Point{}, draw.Over)
	return out
}

// extractAlpha pulls the alpha channel into a packed w*h plane.
func extractAlpha(src *image.RGBA) []uint8 {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			out[y*w+x] = src.Pix[src.PixOffset(b.Min.X+x, b.Min.Y+y)+3]
		}
	}
	return out
}

// colorizeAlpha turns an alpha plane into a premultiplied RGBA silhouette.
func colorizeAlpha(alpha []uint8, w, h int, c [3]float64, scale float64) *image.RGBA {
	if scale < 0 {
		scale = 0
	}
	if scale > 1 {
		scale = 1
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, a := range alpha {
		af := float64(a) / 255 * scale
		if af <= 0 {
			continue
		}
		aa := uint8(af*255 + 0.5)
		out.Pix[i*4] = uint8(c[0] * af)
		out.Pix[i*4+1] = uint8(c[1] * af)
		out.Pix[i*4+2] = uint8(c[2] * af)
		out.Pix[i*4+3] = aa
	}
	return out
}

// blurRGBA applies a separable box blur to every channel. gg layers are
// premultiplied RGBA, so blurring channels directly keeps transparent
// edges dark rather than pulling in background color.
func blurRGBA(src *image.RGBA, radius int) *image.RGBA {
	if radius < 1 {
		return src
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	planes := [4][]uint8{}
	for c := range planes {
		plane := make([]uint8, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				plane[y*w+x] = src.Pix[src.PixOffset(b.Min.X+x, b.Min.Y+y)+c]
			}
		}
		planes[c] = blurPlane(plane, w, h, radius)
	}
	for i := 0; i < w*h; i++ {
		dst.Pix[i*4] = planes[0][i]
		dst.Pix[i*4+1] = planes[1][i]
		dst.Pix[i*4+2] = planes[2][i]
		dst.Pix[i*4+3] = planes[3][i]
	}
	return dst
}

// blurPlane is a two-pass box blur. Edge samples repeat the nearest
// pixel, which keeps the frame border from fading into transparent black.
func blurPlane(src []uint8, w, h, radius int) []uint8 {
	if radius < 1 || w <= 0 || h <= 0 {
		return src
	}
	win := radius*2 + 1
	tmp := make([]uint8, w*h)
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		row := y * w
		sum := 0
		for x := -radius; x <= radius; x++ {
			sum += int(src[row+clampInt(x, 0, w-1)])
		}
		for x := 0; x < w; x++ {
			tmp[row+x] = uint8(sum / win)
			sum -= int(src[row+clampInt(x-radius, 0, w-1)])
			sum += int(src[row+clampInt(x+radius+1, 0, w-1)])
		}
	}
	for x := 0; x < w; x++ {
		sum := 0
		for y := -radius; y <= radius; y++ {
			sum += int(tmp[clampInt(y, 0, h-1)*w+x])
		}
		for y := 0; y < h; y++ {
			out[y*w+x] = uint8(sum / win)
			sum -= int(tmp[clampInt(y-radius, 0, h-1)*w+x])
			sum += int(tmp[clampInt(y+radius+1, 0, h-1)*w+x])
		}
	}
	return out
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
