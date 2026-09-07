package kyfram

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/codetesla51/logos/interpreter"
)

// A declared scene must come back as a Timeline: header set, keyframes
// sorted, props converted. Missing scene() is an error, not silence.
func TestBindings(t *testing.T) {
	src := `
scene(2, 30)
shape("ball", "circle", table{
  "2": table{x: 320.0, color: "#ffffff"},
  "0": table{x: 100.0, color: "#000000"}
})
text("caption", table{
  "0": table{content: "hi", x: 320.0}
})
`
	vm := interpreter.NewInterpreter(interpreter.SandboxConfig{
		AllowFileIO: false, AllowNetwork: false,
		AllowShell: false, AllowExit: false,
	})
	var b Builder
	b.Bind(vm)
	if err := vm.Run(src); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tl, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if tl.Duration != 2 || tl.FPS != 30 {
		t.Fatalf("header = %v/%v, want 2/30", tl.Duration, tl.FPS)
	}
	if len(tl.Entities) != 2 {
		t.Fatalf("entities = %d, want 2", len(tl.Entities))
	}
	ball := tl.Entities[0]
	if ball.Name != "ball" || ball.Kind != "circle" {
		t.Fatalf("ball = %+v", ball)
	}
	if len(ball.Keyframes) != 2 || ball.Keyframes[0].Time != 0 || ball.Keyframes[1].Time != 2 {
		t.Fatalf("ball times unsorted: %+v", ball.Keyframes)
	}
	if ball.Keyframes[0].Props["x"] != 100.0 {
		t.Fatalf("x at t=0 = %v, want 100", ball.Keyframes[0].Props["x"])
	}
	if tl.Entities[1].Kind != "text" {
		t.Fatalf("caption kind = %q, want text", tl.Entities[1].Kind)
	}
}

func TestBindingsNoScene(t *testing.T) {
	vm := interpreter.NewInterpreter(interpreter.SandboxConfig{})
	var b Builder
	b.Bind(vm)
	if err := vm.Run(`shape("a", "circle", table{"0": table{}})`); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := b.Build(); err == nil {
		t.Fatal("Build without scene() should error")
	}
}

// audio() collects tracks with offsets; path-only defaults to zero.
func TestBindingsAudio(t *testing.T) {
	vm := interpreter.NewInterpreter(interpreter.SandboxConfig{})
	var b Builder
	b.Bind(vm)
	src := `
scene(1, 10)
audio("a.mp3")
audio("b.mp3", 1.5)
audio("c.mp3", 0, true)
`
	if err := vm.Run(src); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tl, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(tl.Audio) != 3 {
		t.Fatalf("tracks = %d, want 3", len(tl.Audio))
	}
	if tl.Audio[0].Offset != 0 || tl.Audio[1].Offset != 1.5 {
		t.Fatalf("tracks = %+v", tl.Audio)
	}
	if !tl.Audio[2].Loop {
		t.Fatalf("third track should loop: %+v", tl.Audio[2])
	}
}

// text_file returns file content as a string; missing files fail the run.
func TestBindingsTextFile(t *testing.T) {
	dir := t.TempDir()
	note := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(note, []byte("dear viewer"), 0o644); err != nil {
		t.Fatal(err)
	}

	vm := interpreter.NewInterpreter(interpreter.SandboxConfig{})
	var b Builder
	b.Bind(vm)
	if err := vm.Run(fmt.Sprintf(`let t = text_file(%q)`, note)); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := vm.GetVar("t"); got != "dear viewer" {
		t.Fatalf("t = %v", got)
	}

	vm2 := interpreter.NewInterpreter(interpreter.SandboxConfig{})
	var b2 Builder
	b2.Bind(vm2)
	if err := vm2.Run(`text_file("nope-missing.txt")`); err == nil {
		t.Fatal("missing file should error")
	}
}
