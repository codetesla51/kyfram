package kyfram

import "image"

// transitionModes is every transition a scene() can name, plus aliases
// ("push" == "push-right", "to-black" == "black"). canonical forms:
// cut, fade, black, white, flash, push-{left,right,up,down},
// wipe-{left,right,up,down}, iris, zoom.
var transitionModes = []string{
	"cut", "fade", "black", "to-black", "white", "to-white", "flash",
	"push", "push-left", "push-right", "push-up", "push-down",
	"wipe", "wipe-left", "wipe-right", "wipe-up", "wipe-down",
	"iris", "zoom",
}

func normTransition(mode string) string {
	switch mode {
	case "", "cut", "to-black", "to-white", "push", "wipe":
		switch mode {
		case "to-black":
			return "black"
		case "to-white":
			return "white"
		case "push":
			return "push-right"
		case "wipe":
			return "wipe-right"
		}
	}
	return mode
}

// transitionDuration resolves how long a transition lasts; an explicit
// scene() 4th arg wins, else a per-mode default (flash is quick).
func transitionDuration(mode string, given float64) float64 {
	if given > 0 {
		return given
	}
	if mode == "flash" {
		return 0.2
	}
	return 0.5
}

// applyTransition composites the incoming scene (src layer, transparent
// where unpainted) over the outgoing scene (dst canvas, fully rendered)
// at progress p in [0,1]. Every mode is one pass over the pixels: which
// source pixel shows where is all that differs.
func applyTransition(dst, src *image.RGBA, mode string, p float64, w, h int) {
	if p <= 0 {
		return
	}
	if p > 1 {
		p = 1
	}
	mode = normTransition(mode)
	switch mode {
	case "fade":
		compositeFade(dst, src, p)
	case "black", "white", "flash":
		dip(dst, src, mode, p)
	case "push-left", "push-right", "push-up", "push-down",
		"wipe-left", "wipe-right", "wipe-up", "wipe-down", "iris", "zoom":
		geometric(dst, src, mode, p, w, h)
	}
}

// dip fades the outgoing scene to a solid color, holds a beat, then
// fades the incoming scene in from it (dip to black/white, and the
// white flash — same curve, shorter default duration).
func dip(dst, src *image.RGBA, mode string, p float64) {
	var cr, cg, cb float64
	if mode == "black" {
		cr, cg, cb = 0, 0, 0
	} else {
		cr, cg, cb = 255, 255, 255
	}
	if p <= 0.5 {
		f := p / 0.5 // reach full color exactly at the hold (p=0.5)
		if f > 1 {
			f = 1
		}
		for i := 0; i+3 < len(dst.Pix); i += 4 {
			dst.Pix[i] = uint8(float64(dst.Pix[i])*(1-f) + cr*f)
			dst.Pix[i+1] = uint8(float64(dst.Pix[i+1])*(1-f) + cg*f)
			dst.Pix[i+2] = uint8(float64(dst.Pix[i+2])*(1-f) + cb*f)
		}
		return
	}
	f := (p - 0.55) / 0.45
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	for i := 0; i+3 < len(dst.Pix) && i+3 < len(src.Pix); i += 4 {
		sa := float64(src.Pix[i+3]) / 255
		w := sa * f
		// note: w==0 must still overwrite with the dip color (the hold);
		// only the source alpha can suppress a write entirely
		dst.Pix[i] = uint8(float64(src.Pix[i])*w + cr*(1-w))
		dst.Pix[i+1] = uint8(float64(src.Pix[i+1])*w + cg*(1-w))
		dst.Pix[i+2] = uint8(float64(src.Pix[i+2])*w + cb*(1-w))
		dst.Pix[i+3] = 255
	}
}

// geometric covers slide (push), wipe, iris, and zoom reveals: each
// mode maps the destination pixel to a source pixel (or nothing, which
// lets the outgoing scene underneath show through).
func geometric(dst, src *image.RGBA, mode string, p float64, w, h int) {
	cx, cy := float64(w)/2, float64(h)/2
	maxR := float64(w+h) / 2 // iris radius at p=1
	scale := 0.03 + 0.97*p   // zoom: 3% dot growing to full frame
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx, dy := float64(x), float64(y)
			sx, sy := dx, dy // identity by default
			visible := false
			switch mode {
			case "wipe-right":
				visible = dx < p*float64(w)
			case "wipe-left":
				visible = dx > float64(w)-p*float64(w)
			case "wipe-down":
				visible = dy < p*float64(h)
			case "wipe-up":
				visible = dy > float64(h)-p*float64(h)
			case "iris":
				visible = (dx-cx)*(dx-cx)+(dy-cy)*(dy-cy) <= (p*maxR)*(p*maxR)
			case "zoom":
				sx = cx + (dx-cx)/scale
				sy = cy + (dy-cy)/scale
				visible = sx >= 0 && sx < float64(w) && sy >= 0 && sy < float64(h)
			case "push-right":
				sx = dx - (1-p)*float64(w)
				visible = sx >= 0 && sx < float64(w)
			case "push-left":
				sx = dx + (1-p)*float64(w)
				visible = sx >= 0 && sx < float64(w)
			case "push-down":
				sy = dy - (1-p)*float64(h)
				visible = sy >= 0 && sy < float64(h)
			case "push-up":
				sy = dy + (1-p)*float64(h)
				visible = sy >= 0 && sy < float64(h)
			}
			if !visible {
				continue
			}
			si := (int(sy)*w + int(sx)) * 4
			if si < 0 || si+3 >= len(src.Pix) {
				continue
			}
			sa := src.Pix[si+3]
			if sa == 0 {
				continue
			}
			di := (y*w + x) * 4
			if sa == 255 {
				dst.Pix[di], dst.Pix[di+1], dst.Pix[di+2], dst.Pix[di+3] = src.Pix[si], src.Pix[si+1], src.Pix[si+2], 255
				continue
			}
			// semi-transparent source pixel: over-composite
			a := float64(sa) / 255
			da := float64(dst.Pix[di+3]) / 255
			oa := a + da*(1-a)
			if oa == 0 {
				continue
			}
			dst.Pix[di] = uint8((float64(src.Pix[si])*a + float64(dst.Pix[di])*da*(1-a)) / oa)
			dst.Pix[di+1] = uint8((float64(src.Pix[si+1])*a + float64(dst.Pix[di+1])*da*(1-a)) / oa)
			dst.Pix[di+2] = uint8((float64(src.Pix[si+2])*a + float64(dst.Pix[di+2])*da*(1-a)) / oa)
			dst.Pix[di+3] = uint8(oa*255 + 0.5)
		}
	}
}
