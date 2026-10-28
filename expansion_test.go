package kyfram

import (
	"testing"
)

// A group clip crops every child, including an oversized background rect.
func TestGroupRectClip(t *testing.T) {
	tl := Timeline{
		Width: 200, Height: 200,
		Groups: []Group{{
			Name: "window",
			Keyframes: []Keyframe{{Time: 0, Props: map[string]any{
				"x": 100.0, "y": 100.0,
				"clip": "rect", "clipW": 40.0, "clipH": 40.0,
			}}},
		}},
		Entities: []Entity{{
			Name: "red", Kind: "rect", Group: "window",
			Keyframes: []Keyframe{{Time: 0, Props: map[string]any{
				"x": -100.0, "y": -100.0, "w": 200.0, "h": 200.0, "color": "#ff0000",
			}}},
		}},
	}
	img := RenderFrame(tl, 0).Image()
	r, _, _, _ := img.At(100, 100).RGBA()
	if r>>8 < 200 {
		t.Fatalf("inside clip = red %d, want visible red", r>>8)
	}
	r, _, _, _ = img.At(100, 140).RGBA()
	if r>>8 > 100 {
		t.Fatalf("outside clip = red %d, want background", r>>8)
	}
}

// A path clip uses local coordinates and is translated by clipX/clipY.
func TestGroupPathClip(t *testing.T) {
	tl := Timeline{
		Width: 200, Height: 200,
		Groups: []Group{{
			Name: "window",
			Keyframes: []Keyframe{{Time: 0, Props: map[string]any{
				"x": 100.0, "y": 100.0,
				"clip": "path", "clipD": "M -30 -30 L 30 -30 L 30 30 L -30 30 Z",
			}}},
		}},
		Entities: []Entity{{
			Name: "red", Kind: "rect", Group: "window",
			Keyframes: []Keyframe{{Time: 0, Props: map[string]any{
				"x": -100.0, "y": -100.0, "w": 200.0, "h": 200.0, "color": "#ff0000",
			}}},
		}},
	}
	img := RenderFrame(tl, 0).Image()
	r, _, _, _ := img.At(100, 100).RGBA()
	if r>>8 < 200 {
		t.Fatalf("inside path clip = red %d, want visible red", r>>8)
	}
	r, _, _, _ = img.At(100, 145).RGBA()
	if r>>8 > 100 {
		t.Fatalf("outside path clip = red %d, want background", r>>8)
	}
}

// Shadow is a blurred, offset silhouette of the entity alpha.
func TestShadowOffsetsSilhouette(t *testing.T) {
	tl := Timeline{
		Width: 100, Height: 100,
		Entities: []Entity{{
			Name: "dot", Kind: "circle",
			Keyframes: []Keyframe{{Time: 0, Props: map[string]any{
				"x": 30.0, "y": 30.0, "r": 10.0, "color": "#ffffff",
				"shadowColor": "#ff0000", "shadowBlur": 0.0,
				"shadowX": 20.0, "shadowY": 20.0,
			}}},
		}},
	}
	img := RenderFrame(tl, 0).Image()
	r, g, b, _ := img.At(50, 50).RGBA()
	if r>>8 < 180 || g>>8 > 80 || b>>8 > 80 {
		t.Fatalf("shadow pixel = #%02x%02x%02x, want red", r>>8, g>>8, b>>8)
	}
	r, g, b, _ = img.At(30, 30).RGBA()
	if r>>8 < 200 || g>>8 < 200 || b>>8 < 200 {
		t.Fatalf("entity pixel = #%02x%02x%02x, want white on top of shadow", r>>8, g>>8, b>>8)
	}
}

// Blur spreads alpha beyond the original edge.
func TestBlurSpreadsAlpha(t *testing.T) {
	tl := Timeline{
		Width: 100, Height: 100, Transparent: true,
		Entities: []Entity{{
			Name: "dot", Kind: "circle",
			Keyframes: []Keyframe{{Time: 0, Props: map[string]any{
				"x": 50.0, "y": 50.0, "r": 8.0, "color": "#ffffff", "blur": 6.0,
			}}},
		}},
	}
	img := RenderFrame(tl, 0).Image()
	_, _, _, a := img.At(50, 50).RGBA()
	if a>>8 < 180 {
		t.Fatalf("center alpha = %d, want solid", a>>8)
	}
	_, _, _, a = img.At(50, 62).RGBA()
	if a>>8 == 0 {
		t.Fatal("blurred edge alpha = 0, want some spill")
	}
}

// Glow contributes color outside the source shape.
func TestGlowTintsOutsideShape(t *testing.T) {
	tl := Timeline{
		Width: 100, Height: 100, Transparent: true,
		Entities: []Entity{{
			Name: "dot", Kind: "circle",
			Keyframes: []Keyframe{{Time: 0, Props: map[string]any{
				"x": 50.0, "y": 50.0, "r": 8.0, "color": "#ffffff",
				"glow": "#ff0000", "glowBlur": 10.0,
			}}},
		}},
	}
	img := RenderFrame(tl, 0).Image()
	r, g, b, a := img.At(50, 64).RGBA()
	if a>>8 == 0 {
		t.Fatal("glow edge alpha = 0, want glow spill")
	}
	if r>>8 < 20 || g>>8 > 80 || b>>8 > 80 {
		t.Fatalf("glow pixel = #%02x%02x%02x, want red tint", r>>8, g>>8, b>>8)
	}
}

// letters() supports explicit newlines and offsets each line vertically.
func TestLettersMultiline(t *testing.T) {
	tl, err := sceneScript(t, `
scene(2, 30)
letters("title", "AB\nCD", table{
  "0": table{x: 320, y: 180, size: 40, color: "#ffffff"},
  "1": table{x: 320, y: 180, size: 40, color: "#ffffff"},
}, 0.1)
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ents := tl.Scenes[0].Entities
	if len(ents) != 4 {
		t.Fatalf("entities = %d, want 4 letters", len(ents))
	}
	y0 := ents[0].Keyframes[0].Props["y"].(float64)
	y2 := ents[2].Keyframes[0].Props["y"].(float64)
	x0 := ents[0].Keyframes[0].Props["x"].(float64)
	x1 := ents[1].Keyframes[0].Props["x"].(float64)
	if !(y0 < y2) {
		t.Fatalf("line y = (%v, %v), want first line above second", y0, y2)
	}
	if !(x0 < x1) {
		t.Fatalf("letter x = (%v, %v), want A left of B", x0, x1)
	}
}
