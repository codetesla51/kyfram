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
