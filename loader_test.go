package kyfram

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codetesla51/logos/interpreter"
)

// Local deps inline first with use-lines stripped; std/ lines survive
// for Logos; cycles error out.
func TestLoadScript(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("lib.lgv", "shape(\"dep\", \"circle\", table{\"0\": table{}})\n")
	write("main.lgv", "use \"lib\"\nuse \"std/array\"\nscene(1, 10)\nshape(\"own\", \"circle\", table{\"0\": table{}})\n")

	src, err := LoadScript(filepath.Join(dir, "main.lgv"))
	if err != nil {
		t.Fatalf("LoadScript: %v", err)
	}
	if strings.Contains(src, `use "lib"`) {
		t.Fatal("local use-line survived")
	}
	if !strings.Contains(src, `use "std/array"`) {
		t.Fatal("std use-line stripped, Logos needs it")
	}
	if strings.Index(src, `"dep"`) > strings.Index(src, `"own"`) {
		t.Fatal("dep must inline before main content")
	}

	// End to end: the merged source builds both entities.
	vm := interpreter.NewInterpreter(StdLib(), interpreter.SandboxConfig{})
	var b Builder
	b.Bind(vm)
	if err := vm.Run(src); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tl, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(tl.Entities) != 2 {
		t.Fatalf("entities = %d, want 2", len(tl.Entities))
	}

	// Cycles error instead of hanging.
	write("a.lgv", "use \"b\"\n")
	write("b.lgv", "use \"a\"\n")
	if _, err := LoadScript(filepath.Join(dir, "a.lgv")); err == nil {
		t.Fatal("cycle should error")
	}
}
