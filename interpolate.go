package main

import (
	"fmt"
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
	f := 0.0
	if b.Time > a.Time {
		f = (t - a.Time) / (b.Time - a.Time)
	}

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
			if ac, err := parseHex(as); err == nil {
				if bc, err := parseHex(bs); err == nil {
					return mixHex(ac, bc, f)
				}
			}
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
