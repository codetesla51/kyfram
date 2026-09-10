package kyfram

import (
	"strings"
	"testing"

	"github.com/codetesla51/logos/interpreter"
)

// sceneScript builds a Timeline from source through the normal path.
func sceneScript(t *testing.T, src string) (Timeline, error) {
	t.Helper()
	vm := interpreter.NewInterpreter(interpreter.SandboxConfig{
		AllowFileIO: false, AllowNetwork: false,
		AllowShell: false, AllowExit: false,
	})
	var b Builder
	b.Bind(vm)
	if err := vm.Run(src); err != nil {
		return Timeline{}, err
	}
	return b.Build()
}

// Two scenes, no transition: each scene's content shows only inside its
// own window, and Build() totals the durations.
func TestScenesHardCut(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("bg", "rect", table{
  "0": table{x: 0, y: 0, w: 640, h: 360, color: "#ff0000"},
})
scene(1, 30)
shape("bg", "rect", table{
  "0": table{x: 0, y: 0, w: 640, h: 360, color: "#0000ff"},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if tl.Duration != 2 {
		t.Fatalf("Duration = %v, want 2 (sum)", tl.Duration)
	}
	pix := func(t float64) (uint32, uint32, uint32, uint32) {
		return RenderFrame(tl, t).Image().At(320, 180).RGBA()
	}
	r, _, b, _ := pix(0.5)
	if r>>8 != 0xff || b>>8 != 0x00 {
		t.Fatalf("scene 1 = r%d b%d, want red", r>>8, b>>8)
	}
	r, _, b, _ = pix(1.5)
	if r>>8 != 0x00 || b>>8 != 0xff {
		t.Fatalf("scene 2 = r%d b%d, want blue", r>>8, b>>8)
	}
}

// Scene-local keyframe times are offset to absolute, so t=0.5 inside
// scene 2 means 1.5 absolute.
func TestScenesEntityOffset(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 180, r: 40, color: "#ffffff"},
})
scene(1, 30)
shape("ball", "circle", table{
  "0": table{x: 500, y: 180, r: 40, color: "#ffffff"},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	e := tl.Scenes[1].Entities[0]
	if e.Keyframes[0].Time != 1 {
		t.Fatalf("scene-2 keyframe time = %v, want 1 (offset)", e.Keyframes[0].Time)
	}
	// ball in scene 2 must sit at x=500 at absolute t=1.2
	r, g, b, _ := RenderFrame(tl, 1.2).Image().At(500, 180).RGBA()
	if r>>8 != 0xff || g>>8 != 0xff || b>>8 != 0xff {
		t.Fatalf("scene-2 ball pixel = #%02x%02x%02x, want white", r>>8, g>>8, b>>8)
	}
}

// A fade transition crossfades: mid-window pixel is a mix of both
// scene colors, not either endpoint.
func TestScenesFade(t *testing.T) {
	tl, err := sceneScript(t, `
scene(2, 30)
shape("bg", "rect", table{
  "0": table{x: 0, y: 0, w: 640, h: 360, color: "#ff0000"},
})
scene(2, 30, "fade")
shape("bg", "rect", table{
  "0": table{x: 0, y: 0, w: 640, h: 360, color: "#0000ff"},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	// start of fade window: still fully scene 1 (red)
	r, _, b, _ := RenderFrame(tl, 2.0).Image().At(320, 180).RGBA()
	if r>>8 != 0xff || b>>8 != 0x00 {
		t.Fatalf("fade start = r%d b%d, want red", r>>8, b>>8)
	}
	// middle of fade: mix (~half red, half blue)
	r, _, b, _ = RenderFrame(tl, 2.25).Image().At(320, 180).RGBA()
	if r>>8 > 200 || b>>8 > 200 {
		t.Fatalf("fade middle = r%d b%d, want mix", r>>8, b>>8)
	}
	// exact blend at a plain-bg pixel: #ff0000 over #0000ff at f=0.3
	// (regression: compositeFade once wrapped bytes with *255 into
	// uint8 mod-256, producing scrambled chroma like #54f084)
	r, g, b, _ := RenderFrame(tl, 2.15).Image().At(10, 10).RGBA()
	f := 0.3
	wantR := uint8(0x00*f + 0xff*(1-f)) // red (scene 1) still dominant: 0.7
	wantB := uint8(0xff*f + 0x00*(1-f)) // blue (scene 2) arriving: 0.3
	if r>>8 != uint32(wantR) || b>>8 != uint32(wantB) || g>>8 != 0 {
		t.Fatalf("exact fade = r%d g%d b%d, want r%d b%d", r>>8, g>>8, b>>8, wantR, wantB)
	}
	// end of window: blue wins
	r, _, b, _ = RenderFrame(tl, 2.5).Image().At(320, 180).RGBA()
	if r>>8 != 0x00 || b>>8 != 0xff {
		t.Fatalf("fade end = r%d b%d, want blue", r>>8, b>>8)
	}
}

// Unknown transition strings fail fast at run, not mid-encode.
func TestScenesBadTransition(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
scene(1, 30, "spin")
`)
	if err == nil || !strings.Contains(err.Error(), `unknown transition "spin"`) {
		t.Fatalf("err = %v, want unknown transition reject", err)
	}
}

// A bad kind in scene 2 names the scene in the error.
func TestScenesValidation(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
scene(1, 30)
shape("oops", "circl", table{
  "0": table{x: 1, y: 2},
})
`)
	if err == nil || !strings.Contains(err.Error(), `scene 2 entity "oops"`) {
		t.Fatalf("err = %v, want scene-scoped message", err)
	}
}

// No scene() is still an error.
func TestScenesRequireScene(t *testing.T) {
	_, err := sceneScript(t, `shape("a", "circle", table{"0": table{}})`)
	if err == nil {
		t.Fatal("expected no-scene error")
	}
}
