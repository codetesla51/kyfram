package kyfram

import (
	"github.com/fogleman/gg"
)

// hasClip reports whether props ask for a clip shape the renderer knows.
// "none" and an empty value are treated as no clip, so a clip can be
// switched off in a later keyframe.
func hasClip(props map[string]any) bool {
	switch propStr(props, "clip", "") {
	case "rect", "roundRect", "circle", "path":
		return true
	}
	return false
}

// applyClip intersects the current clip region with the shape declared by
// clip* props. Coordinates are in the caller's current local space:
// rect/roundRect/circle are centered at clipX/clipY, while a path is
// translated by clipX/clipY. It reports whether a clip was applied.
func applyClip(dc *gg.Context, props map[string]any) bool {
	kind := propStr(props, "clip", "")
	cx, cy := propNum(props, "clipX", 0), propNum(props, "clipY", 0)
	switch kind {
	case "rect", "roundRect":
		w := propNum(props, "clipW", 100)
		h := propNum(props, "clipH", 100)
		x, y := cx-w/2, cy-h/2
		if kind == "roundRect" {
			dc.DrawRoundedRectangle(x, y, w, h, propNum(props, "clipRadius", 10))
		} else {
			dc.DrawRectangle(x, y, w, h)
		}
	case "circle":
		dc.DrawCircle(cx, cy, propNum(props, "clipR", 50))
	case "path":
		// Do not wrap this in Push/Pop: Pop restores the path, which
		// would discard the geometry before Clip() consumes it.
		dc.Translate(cx, cy)
		traced := false
		if d := propStr(props, "clipD", ""); d != "" {
			if contours, err := flattenPathD(d); err == nil {
				traceContours(dc, contours, true)
				traced = true
			}
		} else if raw, ok := props["clipPoints"].([]any); ok {
			pts, ok := numberList(raw)
			if ok {
				traceContours(dc, []pathContour{{points: pts, closed: true}}, true)
				traced = true
			}
		}
		dc.Translate(-cx, -cy)
		if !traced {
			return false
		}
	default:
		return false
	}
	// Clip paths are filled with even-odd so holes in imported icons work.
	dc.SetFillRule(gg.FillRuleEvenOdd)
	dc.Clip()
	dc.SetFillRule(gg.FillRuleWinding)
	return true
}

// numberList converts a flat []any to []float64. SVG-style point lists
// arrive through the Logos bridge as []any, not []float64.
func numberList(raw []any) ([]float64, bool) {
	if len(raw) < 4 || len(raw)%2 != 0 {
		return nil, false
	}
	pts := make([]float64, len(raw))
	for i, v := range raw {
		f, ok := toFloat(v)
		if !ok {
			return nil, false
		}
		pts[i] = f
	}
	return pts, true
}
