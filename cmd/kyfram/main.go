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
		printHelp()
		os.Exit(0)
	}
	switch os.Args[1] {
	case "help", "--help", "-h":
		if len(os.Args) == 3 {
			switch os.Args[2] {
			case "render":
				printHelpRender()
			case "view":
				printHelpView()
			default:
				printHelp()
			}
		} else {
			printHelp()
		}
	case "version", "--version", "-v":
		fmt.Println("kyfram v0.1.6")
	case "render":
		if hasHelp(os.Args[2:]) {
			printHelpRender()
			return
		}
		runRender(os.Args[2:])
	case "view":
		if hasHelp(os.Args[2:]) {
			printHelpView()
			return
		}
		runView(os.Args[2:])
	default:
		fmt.Fprintf(os.Stderr, "%s unknown command %q\n\n", bold("kyfram"), os.Args[1])
		printHelp()
		os.Exit(1)
	}
}

func hasHelp(args []string) bool {
	for _, a := range args {
		if a == "--help" || a == "-h" || a == "help" {
			return true
		}
	}
	return false
}

const (
	cReset = "\033[0m"
	cBold  = "\033[1m"
	cDim   = "\033[2m"
	cCyan  = "\033[36m"
	cYell  = "\033[33m"
	cGreen = "\033[32m"
)

func bold(s string) string  { return cBold + s + cReset }
func cyan(s string) string  { return cCyan + s + cReset }
func dim(s string) string   { return cDim + s + cReset }
func yellow(s string) string { return cYell + s + cReset }

func printHelp() {
	fmt.Printf(`%s %s
`, bold(cyan("kyfram")), dim("— keyframe-driven rendering engine"))
	fmt.Printf("  %s\n\n", dim("declare states at times, Go renders to mp4/png via gg + ffmpeg"))
	fmt.Printf("%s\n", bold("USAGE:"))
	fmt.Printf("  %s %s %s\n\n", bold("kyfram"), cyan("<command>"), dim("[options]"))
	fmt.Printf("%s\n", bold("COMMANDS:"))
	fmt.Printf("  %s  %s\n", cyan("render"), "Render a .lgv script to video or image")
	fmt.Printf("  %s    %s\n", cyan("view"), "Live preview with scrub + hot reload")
	fmt.Printf("  %s    %s\n\n", cyan("help"), "Show help (try "+yellow("help render")+")")
	fmt.Printf("%s\n", bold("OPTIONS:"))
	fmt.Printf("  %s, %s    %s\n", yellow("-h"), yellow("--help"), "Show help")
	fmt.Printf("  %s, %s %s\n", yellow("-v"), yellow("--version"), dim("Show version"))
	fmt.Printf("\nRun %s for details.\n", bold("kyfram help <command>"))
}

func printHelpRender() {
	fmt.Printf("%s %s\n", bold(cyan("kyfram render")), dim("— render a script to mp4/png"))
	fmt.Printf("\n%s\n", bold("USAGE:"))
	fmt.Printf("  %s %s %s %s\n", bold("kyfram render"), dim("[options]"), cyan("<script.lgv>"), dim(""))
	fmt.Printf("\n%s\n", bold("OPTIONS:"))
	fmt.Printf("  %s %s   %s (default %s)\n", yellow("--out"), dim("FILE"), "Output path", dim("out.mp4"))
	fmt.Printf("  %s %s    %s\n", yellow("--fps"), dim("N"), "Override scene FPS")
	fmt.Printf("  %s %s  %s\n", yellow("--width"), dim("N"), "Override canvas width")
	fmt.Printf("  %s %s %s\n", yellow("--height"), dim("N"), "Override canvas height")
	fmt.Printf("  %s %s   %s\n", yellow("--font"), dim("FILE"), "Default typeface")
	fmt.Printf("  %s %s    %s (18 high, 23 default)\n", yellow("--crf"), dim("N"), "x264 quality 0-51")
	fmt.Printf("  %s %s %s\n", yellow("--preset"), dim("S"), "x264 preset "+dim("ultrafast..veryslow"))
	fmt.Printf("  %s %s %s\n", yellow("--format"), dim("FMT"), "Output format "+dim("mp4|png"))
	fmt.Printf("  %s %s    %s\n", yellow("--var"), dim("K=V"), "Template var (repeatable)")
	fmt.Printf("  %s, %s      %s\n", yellow("-h"), yellow("--help"), "Show this help")
	fmt.Printf("\n%s\n", bold("EXAMPLES:"))
	fmt.Printf("  %s\n", dim("kyfram render examples/intro.lgv --out out.mp4"))
	fmt.Printf("  %s\n", dim("kyfram render --crf 18 --preset slow examples/intro.lgv"))
	fmt.Printf("  %s\n", dim("kyfram render --format png --out frame.png examples/intro.lgv"))
}

func printHelpView() {
	fmt.Printf("%s %s\n", bold(cyan("kyfram view")), dim("— live preview"))
	fmt.Printf("\n%s\n", bold("USAGE:"))
	fmt.Printf("  %s %s %s\n", bold("kyfram view"), dim("[options]"), cyan("<script.lgv>"))
	fmt.Printf("\n%s\n", bold("OPTIONS:"))
	fmt.Printf("  %s %s   %s (default %s)\n", yellow("--port"), dim("N"), "Preview port", dim("8080"))
	fmt.Printf("  %s %s   %s\n", yellow("--font"), dim("FILE"), "Default typeface")
	fmt.Printf("  %s, %s      %s\n", yellow("-h"), yellow("--help"), "Show this help")
	fmt.Printf("\n%s\n", dim("then open http://localhost:8080"))
}

func usage() { printHelp(); os.Exit(1) }

// varFlags collects repeatable --var key=value flags.
type varFlags []string

func (v *varFlags) String() string { return "" }

func (v *varFlags) Set(s string) error {
	*v = append(*v, s)
	return nil
}

// splitVar cuts key=value on the first equals; values may contain more.
func splitVar(s string) (string, string, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return s[:i], s[i+1:], i > 0
		}
	}
	return "", "", false
}

// runRender renders one script file to a video or single PNG.
func runRender(args []string) {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	out := fs.String("out", "out.mp4", "output file path")
	fps := fs.Int("fps", 0, "override scene fps")
	width := fs.Int("width", 0, "override canvas width")
	height := fs.Int("height", 0, "override canvas height")
	font := fs.String("font", "", "default typeface path")
	crf := fs.Int("crf", 23, "x264 CRF quality 0-51, lower is better (18 high, 28 small)")
	preset := fs.String("preset", "medium", "x264 preset: ultrafast,superfast,veryfast,faster,fast,medium,slow,slower,veryslow")
	format := fs.String("format", "mp4", "output format: mp4 or png (single frame at t=0)")
	var vars varFlags
	fs.Var(&vars, "var", "template variable as key=value (repeatable)")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "flags:", err)
		os.Exit(1)
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: kyfram render [--out f] [--fps n] [--width n] [--height n] [--font f] [--crf n] [--preset s] [--format mp4|png] [--var k=v ...] <script.lgv>")
		os.Exit(1)
	}
	for _, v := range []int{*fps, *width, *height} {
		if v < 0 {
			fmt.Fprintln(os.Stderr, "flags: sizes must be positive")
			os.Exit(1)
		}
	}
	if *crf < 0 || *crf > 51 {
		fmt.Fprintln(os.Stderr, "crf must be 0-51")
		os.Exit(1)
	}
	validPreset := map[string]bool{"ultrafast": true, "superfast": true, "veryfast": true, "faster": true, "fast": true, "medium": true, "slow": true, "slower": true, "veryslow": true}
	if !validPreset[*preset] {
		fmt.Fprintln(os.Stderr, "preset must be one of ultrafast,superfast,veryfast,faster,fast,medium,slow,slower,veryslow")
		os.Exit(1)
	}
	if *format != "mp4" && *format != "png" {
		fmt.Fprintln(os.Stderr, "format must be mp4 or png")
		os.Exit(1)
	}
	source, err := kyfram.LoadScript(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "load:", err)
		os.Exit(1)
	}
	vm := newVM()
	var b kyfram.Builder
	b.Bind(vm)
	// Template variables land in the script env before Run, so one .lgv
	// renders many videos with different names, titles, copy.
	for _, v := range vars {
		key, val, ok := splitVar(v)
		if !ok {
			fmt.Fprintln(os.Stderr, "var: want key=value, got", v)
			os.Exit(1)
		}
		vm.SetVar(key, val)
	}
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
	if *font != "" {
		tl.Font = *font
	}
	if *format == "png" {
		// single frame at t=0, no ffmpeg needed
		if err := kyfram.RenderFrame(tl, 0).SavePNG(*out); err != nil {
			fmt.Fprintln(os.Stderr, "png:", err)
			os.Exit(1)
		}
		fmt.Println("ok:", *out)
		return
	}
	if err := kyfram.EncodeWithOpts(tl, *out, kyfram.EncodeOpts{CRF: *crf, Preset: *preset}); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	fmt.Println("ok:", *out)
}
