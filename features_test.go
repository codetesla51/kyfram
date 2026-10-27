package kyfram

import (
	"math"
	"strings"
	"testing"
)

// letters() must expand to one text entity per non-space glyph, center
// the measured line, and delay each glyph by the stagger.
func TestLettersExpandsAndStaggers(t *testing.T) {
	tl, err := sceneScript(t, `
scene(2, 30)
letters("title", "AB", table{
  "0": table{x: 320, y: 180, size: 50, color: "#ffffff", alpha: 0},
  "1": table{x: 320, y: 180, size: 50, color: "#ffffff", alpha: 1},
}, 0.25)
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ents := tl.Scenes[0].Entities
	if len(ents) != 2 {
		t.Fatalf("entities = %d, want 2 letters", len(ents))
	}
	for i, want := range []string{"A", "B"} {
		if ents[i].Name != "title-"+string(rune('0'+i)) || ents[i].Kind != "text" {
			t.Fatalf("entity %d = %+v, want text title-%d", i, ents[i], i)
		}
		if got := ents[i].Keyframes[0].Props["content"]; got != want {
			t.Fatalf("letter %d content = %v, want %q", i, got, want)
		}
	}
	ax := ents[0].Keyframes[0].Props["x"].(float64)
	bx := ents[1].Keyframes[0].Props["x"].(float64)
	if !(ax < bx) {
		t.Fatalf("letter x = (%v, %v), want A left of B", ax, bx)
	}
	if ents[0].Keyframes[0].Time != 0 || math.Abs(ents[1].Keyframes[0].Time-0.25) > 1e-9 {
		t.Fatalf("staggered times = %v, %v, want 0, 0.25",
			ents[0].Keyframes[0].Time, ents[1].Keyframes[0].Time)
	}
	if ents[0].Keyframes[1].Time != 1 || math.Abs(ents[1].Keyframes[1].Time-1.25) > 1e-9 {
		t.Fatalf("staggered end times = %v, %v, want 1, 1.25",
			ents[0].Keyframes[1].Time, ents[1].Keyframes[1].Time)
	}
}

// trim cuts a line or path by arc length; trimStart offsets the visible
// window. Pixels left of the window stay background.
func TestTrimDrawsVisiblePortion(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("first", "line", table{
  "0": table{x1: 0, y1: 100, x2: 640, y2: 100, width: 20, color: "#ffffff", trim: 0.5}
})
shape("second", "line", table{
  "0": table{x1: 0, y1: 200, x2: 640, y2: 200, width: 20, color: "#ffffff", trimStart: 0.5, trim: 1}
})
shape("curve", "path", table{
  "0": table{points: [100, 300, 300, 300, 500, 300], width: 20, color: "#ff0000", trim: 0.5}
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	img := RenderFrame(tl, 0).Image()
	check := func(name string, x, y int, wantLight bool) {
		t.Helper()
		r, _, _, _ := img.At(x, y).RGBA()
		light := r>>8 > 200
		if light != wantLight {
			t.Fatalf("%s pixel at (%d,%d) light=%v, want %v", name, x, y, light, wantLight)
		}
	}
	check("first visible", 100, 100, true)
	check("first hidden", 500, 100, false)
	check("second hidden", 100, 200, false)
	check("second visible", 500, 200, true)
	check("path visible", 200, 300, true)
	check("path hidden", 400, 300, false)
}

// A child group inherits its parent's translate/rotate, and entities
// inside the child keep local coordinates.
func TestNestedGroupsComposeTransforms(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
group("parent", table{"0": table{x: 320, y: 180, rotate: 90, scale: 1, alpha: 1}})
group("child", table{
  "0": table{x: 40, y: 0, parent: "parent"},
  "1": table{x: 40, y: 0}
})
shape("dot", "rect", table{
  "0": table{x: -5, y: -5, w: 10, h: 10, color: "#ff0000"}
}, "child")
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	img := RenderFrame(tl, 1).Image()
	r, g, b, _ := img.At(320, 220).RGBA()
	if r>>8 < 200 || g>>8 > 80 || b>>8 > 80 {
		t.Fatalf("nested child pixel = #%02x%02x%02x, want red at inherited position", r>>8, g>>8, b>>8)
	}
}

func TestNestedGroupCycleRejected(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
group("a", table{"0": table{parent: "b"}})
group("b", table{"0": table{parent: "a"}})
`)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Build error = %v, want nested group cycle", err)
	}
}

// spring and bounce must pin both endpoints; a wrong curve would leave
// a visible offset at the end of every entrance.
func TestSpringAndBounceEasingEndpoints(t *testing.T) {
	for _, name := range []string{"spring", "bounce", "bounceOut", "bounceIn", "bounceInOut"} {
		if !validEase(name) {
			t.Fatalf("%s not registered as an ease", name)
		}
		if got := applyEase(name, 0); math.Abs(got) > 1e-9 {
			t.Fatalf("%s(0) = %v, want 0", name, got)
		}
		if got := applyEase(name, 1); math.Abs(got-1) > 1e-9 {
			t.Fatalf("%s(1) = %v, want 1", name, got)
		}
	}
}

// rect rotation pivots at the rect center, not its top-left corner.
func TestRectRotationPivotIsCenter(t *testing.T) {
	tl := Timeline{
		Width:  200,
		Height: 200,
		Entities: []Entity{{
			Name: "box",
			Kind: "rect",
			Keyframes: []Keyframe{{Time: 0, Props: map[string]any{
				"x": 50.0, "y": 50.0, "w": 100.0, "h": 100.0,
				"rotate": 45.0, "color": "#ff0000",
			}}},
		}},
	}
	img := RenderFrame(tl, 0).Image()
	centerR, _, _, _ := img.At(100, 100).RGBA()
	if centerR>>8 < 200 {
		t.Fatalf("center pixel = #%02x, want red", centerR>>8)
	}
	cornerR, _, _, _ := img.At(50, 50).RGBA()
	if cornerR>>8 > 80 {
		t.Fatalf("original top-left pixel = #%02x, want background (center pivot)", cornerR>>8)
	}
}
