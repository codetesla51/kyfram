package main

import (
	"fmt"
	"os"

	kyfram "github.com/codetesla51/kyfram"
	"github.com/codetesla51/logos/interpreter"
)

// newVM creates the sandboxed Logos VM used by kyfram.
// Raw *interpreter.Interpreter (not the logos.VM wrapper) because the
// bindings read keyframe tables back as logos.Objects. Sandbox blocks
// file IO, network, shell, exit — kyfram itself provides narrow builtins
// (text_file) where file access is wanted.
func newVM() *interpreter.Interpreter {
	return interpreter.NewInterpreter(interpreter.SandboxConfig{
		AllowFileIO:  false,
		AllowNetwork: false,
		AllowShell:   false,
		AllowExit:    false,
	})
}

func main() {
	if len(os.Args) != 3 || os.Args[1] != "render" {
		fmt.Fprintln(os.Stderr, "usage: kyfram render <script.lgv>")
		os.Exit(1)
	}
	source, err := os.ReadFile(os.Args[2])
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	vm := newVM()
	var b kyfram.Builder
	b.Bind(vm)
	if err := vm.Run(string(source)); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
	tl, err := b.Build()
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		os.Exit(1)
	}
	if err := kyfram.Encode(tl, "out.mp4"); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	fmt.Println("ok: out.mp4")
}
