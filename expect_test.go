package kyfram

import (
	"bytes"
	"strings"
	"testing"
)

// Value checks interpolate at the listed time and pass inside tolerance.
func TestExpectValuePass(t *testing.T) {
	_, err := sceneScript(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 180, r: 40, color: "#e64c40"},
  "2": table{x: 500, y: 180, r: 40, color: "#e64c40"},
})
expect("ball", table{
  "1": table{x: 300.4, y: 180},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}

// A value outside tolerance is a build error with the documented shape.
func TestExpectValueFail(t *testing.T) {
	_, err := sceneScript(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 180, r: 40, color: "#e64c40"},
  "2": table{x: 500, y: 180, r: 40, color: "#e64c40"},
})
expect("ball", table{
  "1": table{x: 320},
})
`)
	if err == nil {
		t.Fatal("Build passed, want expect failure")
	}
	if !strings.Contains(err.Error(), "expect ball @1.0s: x is 300, wanted 320") {
		t.Fatalf("error = %q, want documented expect failure", err)
	}
}

// Values outside the half-unit window still fail.
func TestExpectValueToleranceFails(t *testing.T) {
	_, err := sceneScript(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 180, r: 40},
  "2": table{x: 500, y: 180, r: 40},
})
expect("ball", table{
  "1": table{x: 300.6},
})
`)
	if err == nil || !strings.Contains(err.Error(), "expect ball @1.0s: x is 300, wanted 300.6") {
		t.Fatalf("error = %v, want tolerance failure", err)
	}
}

// Colors compare by value, so "red" matches its hex spelling.
func TestExpectColorPass(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
shape("ball", "circle", table{
  "0": table{x: 320, y: 180, r: 40, color: "#ff0000"},
})
expect("ball", table{
  "0": table{color: "red"},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}

// always/onCanvas passes for a shape that stays visible.
func TestExpectOnCanvasPass(t *testing.T) {
	_, err := sceneScript(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 320, y: 180, r: 40},
})
expect("ball", table{always: "onCanvas"})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}

// always/onCanvas fails as soon as a sample leaves the canvas.
func TestExpectOnCanvasFail(t *testing.T) {
	_, err := sceneScript(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 180, r: 40},
  "2": table{x: 900, y: 180, r: 40},
})
expect("ball", table{always: "onCanvas"})
`)
	if err == nil {
		t.Fatal("Build passed, want onCanvas failure")
	}
	if !strings.Contains(err.Error(), "expect ball @") || !strings.Contains(err.Error(), "off canvas") {
		t.Fatalf("error = %q, want off-canvas expect failure", err)
	}
}

// apart passes when boxes never intersect.
func TestExpectApartPass(t *testing.T) {
	_, err := sceneScript(t, `
scene(2, 30)
shape("a", "circle", table{
  "0": table{x: 100, y: 180, r: 40},
})
shape("b", "circle", table{
  "0": table{x: 400, y: 180, r: 40},
})
expect("a", "b", table{apart: true})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}

// apart fails when the sampled boxes intersect.
func TestExpectApartFail(t *testing.T) {
	_, err := sceneScript(t, `
scene(2, 30)
shape("a", "circle", table{
  "0": table{x: 300, y: 180, r: 50},
})
shape("b", "circle", table{
  "0": table{x: 340, y: 180, r: 50},
})
expect("a", "b", table{apart: true})
`)
	if err == nil {
		t.Fatal("Build passed, want apart failure")
	}
	if !strings.Contains(err.Error(), "expect a, b @") || !strings.Contains(err.Error(), "bounding boxes overlap") {
		t.Fatalf("error = %q, want overlap expect failure", err)
	}
}

// Group translation counts for checked x/y positions.
func TestExpectGroupOffset(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
group("g", table{
  "0": table{x: 420, y: 180},
})
shape("ball", "circle", table{
  "0": table{x: 20, y: 0, r: 40},
}, "g")
expect("ball", table{
  "0": table{x: 440, y: 180},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}

// Scene-local times are offset to absolute time in the failure message.
func TestExpectSceneTimeReportedAbsolute(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
shape("a", "circle", table{
  "0": table{x: 100, y: 180, r: 40},
})
scene(1, 30)
shape("b", "circle", table{
  "0": table{x: 500, y: 180, r: 40},
})
expect("b", table{
  "0": table{x: 100},
})
`)
	if err == nil || !strings.Contains(err.Error(), "expect b @1.0s: x is 500, wanted 100") {
		t.Fatalf("error = %v, want absolute-time expect failure", err)
	}
}

// A scene-local expectation resolves the same name inside its own scene.
func TestExpectSceneLocal(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 180, r: 40},
})
scene(1, 30)
shape("ball", "circle", table{
  "0": table{x: 500, y: 180, r: 40},
})
expect("ball", table{
  "0": table{x: 500},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
}

// expect on a missing entity is a build error.
func TestExpectUnknownEntity(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
shape("ball", "circle", table{
  "0": table{x: 320, y: 180, r: 40},
})
expect("ghost", table{
  "0": table{x: 320},
})
`)
	if err == nil || !strings.Contains(err.Error(), `entity "ghost" not found`) {
		t.Fatalf("error = %v, want unknown entity error", err)
	}
}

// Unknown checked props are caught while the script runs.
func TestExpectUnknownProp(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
shape("ball", "circle", table{
  "0": table{x: 320, y: 180, r: 40},
})
expect("ball", table{
  "0": table{colr: "#ffffff"},
})
`)
	if err == nil || !strings.Contains(err.Error(), `unknown prop "colr"`) {
		t.Fatalf("error = %v, want unknown prop error", err)
	}
}

// A shape that never reaches the canvas is an error, not a warning.
func TestNeverOnCanvas(t *testing.T) {
	_, err := sceneScript(t, `
scene(1, 30)
shape("ghost", "circle", table{
  "0": table{x: 900, y: 180, r: 40},
})
`)
	if err == nil || !strings.Contains(err.Error(), `entity "ghost" (circle) is never on canvas`) {
		t.Fatalf("error = %v, want never-on-canvas error", err)
	}
}

// A shape that leaves the canvas partway is a warning, not an error.
func TestOffCanvasWarns(t *testing.T) {
	tl, err := sceneScript(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 180, r: 40},
  "2": table{x: 900, y: 180, r: 40},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	warnings, err := checkCanvas(tl)
	if err != nil {
		t.Fatalf("checkCanvas: %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], `entity "ball" (circle) is off canvas`) {
		t.Fatalf("warnings = %v, want one off-canvas warning", warnings)
	}
}

// Inspect prints every visible entity and box overlaps without rendering.
func TestInspect(t *testing.T) {
	tl, err := sceneScript(t, `
scene(1, 30)
shape("a", "circle", table{
  "0": table{x: 300, y: 180, r: 50, color: "#e64c40"},
})
shape("b", "circle", table{
  "0": table{x: 340, y: 180, r: 50, color: "#4c80e6"},
})
`)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	var buf bytes.Buffer
	if err := Inspect(tl, 0.5, &buf); err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"t=0.0s", "t=0.5s", "t=1.0s", "a", "b", "OVERLAP a/b"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}
