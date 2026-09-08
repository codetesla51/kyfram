package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/codetesla51/logos/interpreter"
	kyfram "gitlab.com/codetesla51/kyfram"
)

// newVM creates the sandboxed Logos VM used by kyfram.
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
		fmt.Println("kyfram v0.2.0")
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
		fmt.Fprintf(os.Stderr, "kyfram: unknown command %q\n\n", os.Args[1])
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

func printHelp() {
	fmt.Println("kyfram — keyframe-driven rendering engine")
	fmt.Println("  declare states at times, Go renders to mp4/png via gg + ffmpeg")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  kyfram <command> [options]")
	fmt.Println()
	fmt.Println("COMMANDS:")
	fmt.Println("  render    Render a .lgv script to video or image")
	fmt.Println("  view      Live preview with scrub + hot reload")
	fmt.Println("  help      Show help (try \"help render\")")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  -h, --help       Show help")
	fmt.Println("  -v, --version    Show version")
	fmt.Println()
	fmt.Println("Run \"kyfram help <command>\" for details.")
}

func printHelpRender() {
	fmt.Println("kyfram render — render a script to mp4/png")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  kyfram render [options] <script.lgv>")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  --out FILE     Output path (default out.mp4)")
	fmt.Println("  --fps N        Override scene FPS")
	fmt.Println("  --width N      Override canvas width")
	fmt.Println("  --height N     Override canvas height")
	fmt.Println("  --font FILE    Default typeface")
	fmt.Println("  --crf N        x264 quality 0-51 (18 high, 23 default)")
	fmt.Println("  --preset S     x264 preset (ultrafast..veryslow)")
	fmt.Println("  --format FMT   Output format (mp4|png)")
	fmt.Println("  --var K=V      Template var (repeatable)")
	fmt.Println("  -h, --help     Show this help")
	fmt.Println()
	fmt.Println("EXAMPLES:")
	fmt.Println("  kyfram render examples/intro.lgv --out out.mp4")
	fmt.Println("  kyfram render --crf 18 --preset slow examples/intro.lgv")
	fmt.Println("  kyfram render --format png --out frame.png examples/intro.lgv")
}

func printHelpView() {
	fmt.Println("kyfram view — live preview")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  kyfram view [options] <script.lgv>")
	fmt.Println()
	fmt.Println("OPTIONS:")
	fmt.Println("  --port N       Preview port (default 8080)")
	fmt.Println("  --font FILE    Default typeface")
	fmt.Println("  -h, --help     Show this help")
	fmt.Println()
	fmt.Println("then open http://localhost:8080")
}

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
