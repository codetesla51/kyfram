package kyfram

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Interpolate returns the property set at time t (seconds).
// Numbers lerp linearly, hex colors lerp per channel, everything else
// snaps to the earlier keyframe. Out-of-range time holds the nearest
// end instead of extrapolating. Empty input returns an empty map.
func Interpolate(kfs []Keyframe, t float64) map[string]any {
	if len(kfs) == 0 {
		return map[string]any{}
	}
	if len(kfs) == 1 || t <= kfs[0].Time {
		return copyProps(kfs[0].Props)
	}
	if t >= kfs[len(kfs)-1].Time {
		return copyProps(kfs[len(kfs)-1].Props)
	}

	i := 0
	for i < len(kfs)-2 && kfs[i+1].Time <= t {
		i++
	}
	a, b := kfs[i], kfs[i+1]
	// The earlier key owns the segment's curve, so easing reads like
	// intent: "leave here gently" rather than "arrive here gently".
	f := applyEase(a.Ease, spanFrac(a.Time, b.Time, t))

	out := make(map[string]any, len(a.Props))
	for k, av := range a.Props {
		bv, ok := b.Props[k]
		if !ok {
			out[k] = av
			continue
		}
		out[k] = blend(av, bv, f)
	}
	for k, bv := range b.Props {
		if _, ok := out[k]; !ok {
			out[k] = bv
		}
	}
	return out
}

// spanFrac is how far t sits between two keyframe times: 0 at the
// earlier key, 1 at the later. Duplicate times hold the earlier value
// instead of dividing by zero.
func spanFrac(t0, t1, t float64) float64 {
	if t1 > t0 {
		return (t - t0) / (t1 - t0)
	}
	return 0
}

// easeNames lists every curve applyEase understands. Bindings validate
// against this so scripts fail fast on typos instead of silently linear.
var easeNames = []string{
	"linear", "easeIn", "easeOut", "easeInOut",
	"cubicIn", "cubicOut", "cubicInOut", "sine",
}

// validEase reports whether name is a known curve.
func validEase(name string) bool {
	for _, e := range easeNames {
		if e == name {
			return true
		}
	}
	return false
}

// applyEase reshapes the linear fraction into a named curve. Unknown
// names fall back to linear; bindings reject them earlier so this is
// only a backstop for hand-built timelines.
func applyEase(name string, f float64) float64 {
	switch name {
	case "", "linear":
		return f
	case "easeIn":
		return f * f
	case "easeOut":
		return 1 - (1-f)*(1-f)
	case "easeInOut":
		if f < 0.5 {
			return 2 * f * f
		}
		return 1 - 2*(1-f)*(1-f)
	case "cubicIn":
		return f * f * f
	case "cubicOut":
		return 1 - (1-f)*(1-f)*(1-f)
	case "cubicInOut":
		if f < 0.5 {
			return 4 * f * f * f
		}
		return 1 - 4*(1-f)*(1-f)*(1-f)
	case "sine":
		return 0.5 - 0.5*math.Cos(math.Pi*f)
	}
	return f
}

// blend picks per-type combination: numbers and colors lerp, the rest
// snap to the earlier value so text/fonts never produce garbage halves.
func blend(a, b any, f float64) any {
	if af, ok := toFloat(a); ok {
		if bf, ok := toFloat(b); ok {
			return af + (bf-af)*f
		}
	}
	if as, ok := a.(string); ok {
		if bs, ok := b.(string); ok {
			if ac, err := parseColor(as); err == nil {
				if bc, err := parseColor(bs); err == nil {
					return mixHex(ac, bc, f)
				}
			}
		}
	}
	// Same-length numeric arrays morph elementwise, so point lists
	// tween instead of jumping; anything else snaps to the earlier.
	if as, ok := a.([]any); ok {
		if bs, ok := b.([]any); ok && len(as) == len(bs) {
			out := make([]any, len(as))
			for i := range as {
				af, aok := toFloat(as[i])
				bf, bok := toFloat(bs[i])
				if !aok || !bok {
					return a
				}
				out[i] = af + (bf-af)*f
			}
			return out
		}
	}
	return a
}

// toFloat accepts every numeric shape hand-built Timelines and the
// Logos bridge produce, so callers never type-switch twice.
func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// colorNames maps friendly names to hex so scripts read "gold" instead
// of "#e6c040". Names resolve before blending, so they lerp exactly
// like hex. Case-insensitive; unknown strings are not colors.
var colorNames = map[string]string{
	"black": "#000000", "white": "#ffffff",
	"red": "#ff0000", "green": "#00ff00", "blue": "#0000ff",
	"yellow": "#ffff00", "cyan": "#00ffff", "magenta": "#ff00ff",
	"orange": "#ffa500", "purple": "#800080", "pink": "#ffc0cb",
	"teal": "#008080", "navy": "#000080", "gray": "#808080",
	"grey": "#808080", "dark": "#14141c", "gold": "#e6c040",
	"coral": "#e64c40", "sky": "#40a0e6", "mint": "#40e680",
	"lavender": "#b080e6", "slate": "#404040", "silver": "#888888",
}

// parseColor reads #rrggbb, #rgb, or a color name into 0-255 channels.
func parseColor(s string) ([3]float64, error) {
	if hex, ok := colorNames[strings.ToLower(strings.TrimSpace(s))]; ok {
		s = hex
	}
	return parseHex(s)
}

// parseHex reads #rrggbb and #rgb into 0-255 channels.
func parseHex(s string) ([3]float64, error) {
	var c [3]float64
	h := strings.TrimPrefix(strings.ToLower(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return c, fmt.Errorf("bad hex %q", s)
	}
	for i := range 3 {
		n, err := strconv.ParseUint(h[i*2:i*2+2], 16, 8)
		if err != nil {
			return c, err
		}
		c[i] = float64(n)
	}
	return c, nil
}

// mixHex lerps each channel so red-to-blue passes through purple,
// never through a meaningless string midpoint.
func mixHex(a, b [3]float64, f float64) string {
	return fmt.Sprintf("#%02x%02x%02x",
		uint8(a[0]+(b[0]-a[0])*f+0.5),
		uint8(a[1]+(b[1]-a[1])*f+0.5),
		uint8(a[2]+(b[2]-a[2])*f+0.5))
}

// copyProps duplicates a prop map so renders can't mutate keyframes.
func copyProps(p map[string]any) map[string]any {
	out := make(map[string]any, len(p))
	for k, v := range p {
		out[k] = v
	}
	return out
}
