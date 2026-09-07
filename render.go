package kyfram

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fogleman/gg"
)

const (
	frameW = 640
	frameH = 360

	// Only font proven in step 1; configurable fonts arrive with the
	// text vocabulary (step 8). CLI flags own the size later (step 10).
	frameFont = "/usr/share/fonts/noto/NotoSans-Regular.ttf"
)

// RenderFrame paints every entity at time t onto one canvas: resolve
// values through Interpolate, then dispatch on Kind. Unknown kinds are
// skipped so one experimental entity can't kill the whole frame.
func RenderFrame(tl Timeline, t float64) *gg.Context {
	dc := gg.NewContext(frameW, frameH)
	dc.SetRGB(0.1, 0.1, 0.12)
	dc.Clear()

	for _, e := range tl.Entities {
		props := Interpolate(e.Keyframes, t)
		switch e.Kind {
		case "circle":
			dc.SetHexColor(propStr(props, "color", "#ffffff"))
			dc.DrawCircle(
				propNum(props, "x", frameW/2),
				propNum(props, "y", frameH/2),
				propNum(props, "r", 50),
			)
			dc.Fill()
		case "text":
			if err := dc.LoadFontFace(frameFont, propNum(props, "size", 28)); err != nil {
				continue
			}
			dc.SetHexColor(propStr(props, "color", "#ffffff"))
			dc.DrawStringAnchored(
				propStr(props, "content", ""),
				propNum(props, "x", frameW/2),
				propNum(props, "y", frameH/2),
				0.5, 0.5,
			)
		}
	}
	return dc
}

// RenderSequence renders every frame of tl into dir as frame_%04d.png:
// frame count is Duration*FPS, frame i sits at t=i/FPS. Zero-padded
// names keep alphabetical order equal to time order. Failures name the
// frame index so a bad keyframe is findable without re-running blind.
func RenderSequence(tl Timeline, dir string) error {
	if tl.FPS <= 0 {
		return fmt.Errorf("fps %d: must be positive", tl.FPS)
	}
	n := int(tl.Duration*float64(tl.FPS) + 0.5)
	if n < 1 {
		n = 1
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	for i := range n {
		if err := RenderFrame(tl, float64(i)/float64(tl.FPS)).SavePNG(filepath.Join(dir, fmt.Sprintf("frame_%04d.png", i))); err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
	}
	return nil
}

// propNum reads a numeric prop with a default, so entities missing a
// field still draw instead of collapsing to zero.
func propNum(props map[string]any, key string, def float64) float64 {
	if v, ok := props[key]; ok {
		if f, ok := toFloat(v); ok {
			return f
		}
	}
	return def
}

// propStr reads a string prop with a default, so a missing color never
// hands SetHexColor an empty string.
func propStr(props map[string]any, key, def string) string {
	if v, ok := props[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}
