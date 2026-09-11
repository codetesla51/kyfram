package kyfram

import (
	"strings"
	"testing"

	"github.com/codetesla51/logos/interpreter"
)

// buildErr runs source through the normal script path and returns the
// Build() error (or "" if build succeeded).
func buildErr(t *testing.T, src string) string {
	t.Helper()
	vm := interpreter.NewInterpreter(interpreter.SandboxConfig{
		AllowFileIO: false, AllowNetwork: false,
		AllowShell: false, AllowExit: false,
	})
	var b Builder
	b.Bind(vm)
	if err := vm.Run(src); err != nil {
		return err.Error()
	}
	_, err := b.Build()
	if err == nil {
		return ""
	}
	return err.Error()
}

// Validation rejects unknown kinds with a fuzzy suggestion.
func TestBuildBadKind(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
shape("ball", "circl", table{
  "0": table{x: 100, y: 160, r: 80, color: "#e64c40"},
})
`)
	if !strings.Contains(err, `entity "ball"`) {
		t.Fatalf("error missing entity name: %q", err)
	}
	if !strings.Contains(err, `did you mean "circle"`) {
		t.Fatalf("error missing fuzzy suggestion: %q", err)
	}
}

// Validation rejects unknown props with a suggestion.
func TestBuildBadProp(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 160, r: 80, colr: "#e64c40"},
})
`)
	if !strings.Contains(err, `entity "ball"`) || !strings.Contains(err, `unknown prop "colr"`) {
		t.Fatalf("error = %q", err)
	}
	if !strings.Contains(err, `did you mean "color"`) {
		t.Fatalf("error missing fuzzy suggestion: %q", err)
	}
}

// Validation rejects dangling group references.
func TestBuildDanglingGroup(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 160, r: 80, color: "#e64c40"},
}, "missing_group")
`)
	if !strings.Contains(err, `group "missing_group" does not exist`) {
		t.Fatalf("error = %q", err)
	}
}

// A valid script still builds cleanly.
func TestBuildValid(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
group("g", table{
  "0": table{x: 0, y: 0},
})
shape("ball", "circle", table{
  "0": table{x: 100, y: 160, r: 80, color: "#e64c40"},
}, "g")
`)
	if err != "" {
		t.Fatalf("valid script failed: %q", err)
	}
}

// Validation rejects wrong-typed values that the renderer would
// silently replace with defaults.
func TestBuildBadValueType(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: "left", y: 160, r: 80, color: "#e64c40"},
})
`)
	if !strings.Contains(err, `prop "x"`) || !strings.Contains(err, "must be a number") {
		t.Fatalf("error = %q", err)
	}
}

// Validation rejects unparseable colors with a fuzzy suggestion.
func TestBuildBadColor(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
shape("ball", "circle", table{
  "0": table{x: 100, y: 160, r: 80, color: "gren"},
})
`)
	if !strings.Contains(err, `invalid color "gren"`) {
		t.Fatalf("error = %q", err)
	}
	if !strings.Contains(err, `did you mean "green"`) {
		t.Fatalf("error missing fuzzy suggestion: %q", err)
	}
}

// Validation rejects string enums outside the renderer vocabulary.
func TestBuildBadEnum(t *testing.T) {
	err := buildErr(t, `
scene(0.1, 30)
shape("curve", "path", table{
  "0": table{points: [0, 0, 10, 10], width: 7, cap: "rounded", color: "#ffffff"},
})
`)
	if !strings.Contains(err, `invalid cap "rounded"`) {
		t.Fatalf("error = %q", err)
	}
}

// Validation stats image assets instead of silently skipping them.
func TestBuildMissingImage(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
shape("pic", "image", table{
  "0": table{path: "examples/assets/ghost.png", x: 320, y: 180},
})
`)
	if !strings.Contains(err, `image file "examples/assets/ghost.png" not found`) {
		t.Fatalf("error = %q", err)
	}
}

// An image without path can never draw, so it fails fast.
func TestBuildImageNoPath(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
shape("pic", "image", table{
  "0": table{x: 320, y: 180},
})
`)
	if !strings.Contains(err, `needs a "path" prop`) {
		t.Fatalf("error = %q", err)
	}
}

// Validation stats explicit font files instead of silently dropping text.
func TestBuildMissingFont(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
text("caption", table{
  "0": table{content: "hi", x: 320, y: 300, size: 28, font: "examples/assets/fonts/Nope.ttf"},
})
`)
	if !strings.Contains(err, `font file "examples/assets/fonts/Nope.ttf" not found`) {
		t.Fatalf("error = %q", err)
	}
}

// Real assets still build cleanly.
func TestBuildValidAssets(t *testing.T) {
	err := buildErr(t, `
scene(2, 30)
shape("pic", "image", table{
  "0": table{path: "examples/assets/ball.png", x: 320, y: 180},
})
text("caption", table{
  "0": table{content: "hi", x: 320, y: 300, size: 28, font: "examples/assets/fonts/Geist-Bold.ttf"},
})
`)
	if err != "" {
		t.Fatalf("valid assets failed: %q", err)
	}
}
