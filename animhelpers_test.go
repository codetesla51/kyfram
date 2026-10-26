package kyfram

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestFadeInHelper(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("ball", "circle", fadeIn(table{
  x: 100, y: 100, r: 20, color: "#ffffff", alpha: 0.8
}, 0.2, 0.5))
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	kfs := tl.Scenes[0].Entities[0].Keyframes
	if len(kfs) != 2 {
		t.Fatalf("keyframes = %d, want 2", len(kfs))
	}
	if kfs[0].Time != 0.2 || kfs[1].Time != 0.7 {
		t.Fatalf("times = %v, %v; want 0.2, 0.7", kfs[0].Time, kfs[1].Time)
	}
	if kfs[0].Ease != "cubicOut" {
		t.Fatalf("first ease = %q, want cubicOut", kfs[0].Ease)
	}
	if kfs[0].Props["alpha"] != 0.0 || kfs[1].Props["alpha"] != 0.8 {
		t.Fatalf("alpha = %v -> %v, want 0 -> 0.8", kfs[0].Props["alpha"], kfs[1].Props["alpha"])
	}
	if x, _ := toFloat(kfs[0].Props["x"]); x != 100.0 {
		t.Fatalf("x = %v, want 100", kfs[0].Props["x"])
	}
}

func TestSlideInHelper(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("card", "roundRect", slideIn(table{
  x: 100, y: 50, w: 40, h: 30, color: "#ffffff"
}, 0.1, 0.4, "right", 80))
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	kfs := tl.Scenes[0].Entities[0].Keyframes
	if len(kfs) != 2 {
		t.Fatalf("keyframes = %d, want 2", len(kfs))
	}
	if kfs[0].Time != 0.1 || kfs[1].Time != 0.5 {
		t.Fatalf("times = %v, %v; want 0.1, 0.5", kfs[0].Time, kfs[1].Time)
	}
	x0, _ := toFloat(kfs[0].Props["x"])
	x1, _ := toFloat(kfs[1].Props["x"])
	if x0 != 180.0 || x1 != 100.0 {
		t.Fatalf("x = %v -> %v, want 180 -> 100", x0, x1)
	}
	y0, _ := toFloat(kfs[0].Props["y"])
	y1, _ := toFloat(kfs[1].Props["y"])
	if y0 != 50.0 || y1 != 50.0 {
		t.Fatalf("y moved: %v -> %v", y0, y1)
	}
	if kfs[0].Ease != "cubicOut" {
		t.Fatalf("first ease = %q, want cubicOut", kfs[0].Ease)
	}
}

func TestSlideInHelperDefaultsLeft(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("card", "roundRect", slideIn(table{
  x: 100, y: 50, w: 40, h: 30, color: "#ffffff"
}, 0, 0.2))
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	kfs := tl.Scenes[0].Entities[0].Keyframes
	if kfs[0].Props["x"] != 36.0 {
		t.Fatalf("start x = %v, want 36 (100-64)", kfs[0].Props["x"])
	}
}

func TestSlideInHelperDirections(t *testing.T) {
	tests := []struct {
		dir string
		x   float64
		y   float64
	}{
		{"left", 36, 50},
		{"right", 164, 50},
		{"up", 100, -14},
		{"down", 100, 114},
	}
	for _, tt := range tests {
		t.Run(tt.dir, func(t *testing.T) {
			tl, err := sceneScript(t, fmt.Sprintf(`
scene(1, 30)
shape("card", "roundRect", slideIn(table{
  x: 100, y: 50, w: 40, h: 30, color: "#ffffff"
}, 0, 0.2, %q))
`, tt.dir))
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			kfs := tl.Scenes[0].Entities[0].Keyframes
			x0, _ := toFloat(kfs[0].Props["x"])
			y0, _ := toFloat(kfs[0].Props["y"])
			if x0 != tt.x || y0 != tt.y {
				t.Fatalf("start = (%v, %v), want (%v, %v)", x0, y0, tt.x, tt.y)
			}
			x1, _ := toFloat(kfs[1].Props["x"])
			y1, _ := toFloat(kfs[1].Props["y"])
			if x1 != 100.0 || y1 != 50.0 {
				t.Fatalf("end = (%v, %v), want (100, 50)", x1, y1)
			}
		})
	}
}

func TestPopInHelper(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("badge", "circle", popIn(table{
  x: 320, y: 180, r: 30, color: "#ffffff"
}, 0, 0.5))
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	kfs := tl.Scenes[0].Entities[0].Keyframes
	if len(kfs) != 3 {
		t.Fatalf("keyframes = %d, want 3", len(kfs))
	}
	if kfs[0].Time != 0 || math.Abs(kfs[1].Time-0.325) > 0.0001 || kfs[2].Time != 0.5 {
		t.Fatalf("times = %v, %v, %v", kfs[0].Time, kfs[1].Time, kfs[2].Time)
	}
	wantScale := []float64{0.8, 1.05, 1.0}
	for i, want := range wantScale {
		got, _ := toFloat(kfs[i].Props["scale"])
		if math.Abs(got-want) > 0.0001 {
			t.Fatalf("scale[%d] = %v, want %v", i, got, want)
		}
	}
	wantAlpha := []float64{0, 1, 1}
	for i, want := range wantAlpha {
		got, _ := toFloat(kfs[i].Props["alpha"])
		if math.Abs(got-want) > 0.0001 {
			t.Fatalf("alpha[%d] = %v, want %v", i, got, want)
		}
	}
	if kfs[0].Ease != "cubicOut" || kfs[1].Ease != "cubicInOut" {
		t.Fatalf("eases = %q, %q", kfs[0].Ease, kfs[1].Ease)
	}
}

func TestFadeInHelperRenders(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("bg", "rect", table{
  "0": table{x: 0, y: 0, w: 640, h: 360, color: "#000000"}
})
shape("dot", "circle", fadeIn(table{
  x: 320, y: 180, r: 80, color: "#ffffff"
}, 0, 1))
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	r0, _, _, _ := RenderFrame(tl, 0).Image().At(320, 180).RGBA()
	if r0>>8 != 0 {
		t.Fatalf("at t=0 center = %d, want black", r0>>8)
	}
	r1, _, _, _ := RenderFrame(tl, 1).Image().At(320, 180).RGBA()
	if r1>>8 != 0xff {
		t.Fatalf("at t=1 center = %d, want white", r1>>8)
	}
}

func TestHelpersLeavePropsUntouched(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
let p = table{x: 100, y: 100, r: 20, color: "#ffffff", alpha: 0.5}
shape("a", "circle", fadeIn(p, 0, 0.2))
shape("b", "circle", table{"0": p})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ents := tl.Scenes[0].Entities
	if got := ents[0].Keyframes[0].Props["alpha"]; got != 0.0 {
		t.Fatalf("fade start alpha = %v, want 0", got)
	}
	if got := ents[1].Keyframes[0].Props["alpha"]; got != 0.5 {
		t.Fatalf("shared props alpha = %v, want 0.5", got)
	}
}

func TestHelperErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "bad duration",
			src:  `scene(1, 30) shape("a", "circle", fadeIn(table{x: 1, y: 1, r: 1, color: "#fff"}, 0, 0))`,
			want: "duration must be greater than zero",
		},
		{
			name: "negative start",
			src:  `scene(1, 30) shape("a", "circle", fadeIn(table{x: 1, y: 1, r: 1, color: "#fff"}, -0.1))`,
			want: "start time must not be negative",
		},
		{
			name: "props not table",
			src:  `scene(1, 30) shape("a", "circle", fadeIn(1))`,
			want: "first argument must be a props table",
		},
		{
			name: "too many args",
			src:  `scene(1, 30) shape("a", "circle", fadeIn(table{x: 1, y: 1, r: 1, color: "#fff"}, 0, 0.2, 1))`,
			want: "got 4 arguments",
		},
		{
			name: "missing y",
			src:  `scene(1, 30) shape("a", "circle", slideIn(table{x: 1, r: 1, color: "#fff"}))`,
			want: `props must include "y"`,
		},
		{
			name: "bad direction",
			src:  `scene(1, 30) shape("a", "circle", slideIn(table{x: 1, y: 1, r: 1, color: "#fff"}, 0, 0.2, "sideways"))`,
			want: "unknown direction",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := buildErr(t, tt.src)
			if !strings.Contains(err, tt.want) {
				t.Fatalf("error = %q, want substring %q", err, tt.want)
			}
		})
	}
}
