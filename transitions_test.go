package kyfram

import (
	"fmt"
	"strings"
	"testing"
)

// Pair of fullscreen solid scenes (red 1s, blue 1s), middle scene gets
// the given transition/duration.
const twoSceneTpl = `
scene(1, 30)
shape("bg", "rect", table{
  "0": table{x: 0, y: 0, w: 640, h: 360, color: "#ff0000"},
})
scene(1, 30, "%s")
shape("bg", "rect", table{
  "0": table{x: 0, y: 0, w: 640, h: 360, color: "#0000ff"},
})
`

func twoScene(t *testing.T, tr string) Timeline {
	t.Helper()
	tl, err := sceneScript(t, sprintf(twoSceneTpl, tr))
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return tl
}

func px(tl Timeline, t, x, y float64) (uint8, uint8, uint8) {
	r, g, b, _ := RenderFrame(tl, t).Image().At(int(x), int(y)).RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}

// The transition runs over 0.5s from t=1.0; p = (t-1)/0.6 interpolates
// the mid-window probe point t = 1.3 (p≈0.5).
func midPx(tl Timeline, x, y float64) (uint8, uint8, uint8) { return px(tl, 1.3, x, y) }

func isRed(r, g, b uint8) bool  { return r > 200 && g < 60 && b < 60 }
func isBlue(r, g, b uint8) bool { return b > 200 && r < 60 && g < 60 }

func TestTransitionWipeRight(t *testing.T) {
	tl := twoScene(t, "wipe-right")
	// at p=0.5 the left half is already blue, right half still red
	if r, g, b := midPx(tl, 20, 180); !isBlue(r, g, b) {
		t.Fatalf("wipe left edge = %d %d %d, want blue", r, g, b)
	}
	if r, g, b := midPx(tl, 620, 180); !isRed(r, g, b) {
		t.Fatalf("wipe right edge = %d %d %d, want red", r, g, b)
	}
}

func TestTransitionPushRight(t *testing.T) {
	tl := twoScene(t, "push-right")
	// blue slides in from the right at p≈0.5: right strip blue, left red
	if r, g, b := midPx(tl, 620, 180); !isBlue(r, g, b) {
		t.Fatalf("push right edge = %d %d %d, want blue", r, g, b)
	}
	if r, g, b := midPx(tl, 20, 180); !isRed(r, g, b) {
		t.Fatalf("push left edge = %d %d %d, want red", r, g, b)
	}
}

func TestTransitionIris(t *testing.T) {
	tl := twoScene(t, "iris")
	// growing circle from center: middle blue, corners still red
	if r, g, b := midPx(tl, 320, 180); !isBlue(r, g, b) {
		t.Fatalf("iris center = %d %d %d, want blue", r, g, b)
	}
	if r, g, b := midPx(tl, 5, 5); !isRed(r, g, b) {
		t.Fatalf("iris corner = %d %d %d, want red", r, g, b)
	}
}

func TestTransitionZoom(t *testing.T) {
	tl := twoScene(t, "zoom")
	// scale grows from center: center blue, far corner still red
	if r, g, b := midPx(tl, 320, 180); !isBlue(r, g, b) {
		t.Fatalf("zoom center = %d %d %d, want blue", r, g, b)
	}
	if r, g, b := midPx(tl, 10, 350); !isRed(r, g, b) {
		t.Fatalf("zoom corner = %d %d %d, want red", r, g, b)
	}
}

func TestTransitionDipBlack(t *testing.T) {
	tl := twoScene(t, "black")
	// dip midpoint (p=0.5, t=1.25): near black
	r, g, b := px(tl, 1.25, 320, 180)
	if r > 40 || g > 40 || b > 40 {
		t.Fatalf("dip mid = %d %d %d, want near black", r, g, b)
	}
	// just before the dip old scene still shows
	if r, g, b := px(tl, 1.05, 320, 180); !isRed(r, g, b) {
		t.Fatalf("dip early = %d %d %d, want red", r, g, b)
	}
}

func TestTransitionFlash(t *testing.T) {
	tl := twoScene(t, "flash")
	// flash peaks bright white ~p=0.5 of its 0.2s window (t=1.1)
	r, g, b := px(tl, 1.1, 320, 180)
	if r < 220 || g < 220 || b < 220 {
		t.Fatalf("flash peak = %d %d %d, want near white", r, g, b)
	}
	// after the flash the new scene is in
	if r, g, b := px(tl, 1.25, 320, 180); !isBlue(r, g, b) {
		t.Fatalf("post-flash = %d %d %d, want blue", r, g, b)
	}
}

func TestTransitionAliasesAndDuration(t *testing.T) {
	// bare "push" == push-right; custom duration 0.8 makes the window longer
	tl, err := sceneScript(t, `
scene(1, 30)
shape("bg", "rect", table{"0": table{x: 0, y: 0, w: 640, h: 360, color: "#ff0000"}})
scene(1, 30, "push", 0.8)
shape("bg", "rect", table{"0": table{x: 0, y: 0, w: 640, h: 360, color: "#0000ff"}})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// at 1.5s (p=0.625) the blue has covered 62.5% of the way in from the right
	if r, g, b := px(tl, 1.5, 320, 180); !isBlue(r, g, b) {
		t.Fatalf("custom-dur push = %d %d %d, want blue at center", r, g, b)
	}
	// 1.9s is still inside 0.8s window: fully blue except tiny right edge
	if r, g, b := px(tl, 1.9, 20, 180); !isBlue(r, g, b) {
		t.Fatalf("late window = %d %d %d, want blue", r, g, b)
	}
}

func TestTransitionUnknown(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
scene(1, 30, "spin")
`)
	if err == nil || !strings.Contains(err.Error(), `unknown transition "spin"`) {
		t.Fatalf("err = %v, want unknown transition reject", err)
	}
}

func TestTransitionDurationDefault(t *testing.T) {
	if d := transitionDuration("black", 0); d != 0.5 {
		t.Fatalf("black default = %v, want 0.5", d)
	}
	if d := transitionDuration("flash", 0); d != 0.2 {
		t.Fatalf("flash default = %v, want 0.2", d)
	}
	if d := transitionDuration("flash", 1); d != 1 {
		t.Fatalf("explicit flash = %v, want 1", d)
	}
}
func sprintf(f string, a ...any) string { return fmt.Sprintf(f, a...) }
