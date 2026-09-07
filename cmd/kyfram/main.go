package main

import (
	"flag"
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
	return interpreter.NewInterpreter(kyfram.StdLib(), interpreter.SandboxConfig{
		AllowFileIO:  false,
		AllowNetwork: false,
		AllowShell:   false,
		AllowExit:    false,
	})
}

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "render":
		runRender(os.Args[2:])
	case "view":
		runView(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: kyfram render [--out f] [--fps n] [--width n] [--height n] <script.lgv>")
	fmt.Fprintln(os.Stderr, "       kyfram view [--port n] <script.lgv>")
	os.Exit(1)
}

// runRender renders one script file to a video.
func runRender(args []string) {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	out := fs.String("out", "out.mp4", "output video path")
	fps := fs.Int("fps", 0, "override scene fps")
	width := fs.Int("width", 0, "override canvas width")
	height := fs.Int("height", 0, "override canvas height")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "flags:", err)
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: kyfram render [--out f] [--fps n] [--width n] [--height n] <script.lgv>")
		os.Exit(1)
	}
	for _, v := range []int{*fps, *width, *height} {
		if v < 0 {
			fmt.Fprintln(os.Stderr, "flags: sizes must be positive")
			os.Exit(1)
		}
	}
	source, err := kyfram.LoadScript(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
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
	if *fps > 0 {
		tl.FPS = *fps
	}
	if *width > 0 {
		tl.Width = *width
	}
	if *height > 0 {
		tl.Height = *height
	}
	if err := kyfram.Encode(tl, *out); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	fmt.Println("ok:", *out)
}
