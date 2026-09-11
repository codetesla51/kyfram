package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/codetesla51/logos/interpreter"
	"github.com/spf13/cobra"
	kyfram "gitlab.com/codetesla51/kyfram"
)

// version is the only copy — help text and --version both read it.
const version = "v0.2.0"

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
	root := &cobra.Command{
		Use:   "kyfram",
		Short: "Keyframe-driven rendering engine",
		Run: func(cmd *cobra.Command, _ []string) {
			if v, _ := cmd.Flags().GetBool("version"); v {
				fmt.Println("kyfram", version)
				return
			}
			printHelp()
		},
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.Flags().BoolP("version", "v", false, "Show version")
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		switch cmd.Name() {
		case "render":
			printHelpRender()
		case "view":
			printHelpView()
		case "new":
			printHelpNew()
		default:
			printHelp()
		}
	})
	root.SetHelpCommand(helpCommand())
	root.AddCommand(renderCommand(), viewCommand(), newCommand())
	if err := root.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "kyfram: %v\n\n", err)
		printHelp()
		os.Exit(1)
	}
}

// helpCommand reproduces "kyfram help [render|view|new]".
func helpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Show help (try \"help render\")",
		Run: func(_ *cobra.Command, args []string) {
			if len(args) == 1 {
				switch args[0] {
				case "render":
					printHelpRender()
					return
				case "view":
					printHelpView()
					return
				case "new":
					printHelpNew()
					return
				}
			}
			printHelp()
		},
	}
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
	fmt.Println("  new       Scaffold a starter project")
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
	fmt.Println("  kyfram render --out out.mp4 examples/intro.lgv")
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

func printHelpNew() {
	fmt.Println("kyfram new — scaffold a starter project")
	fmt.Println()
	fmt.Println("USAGE:")
	fmt.Println("  kyfram new <name>")
	fmt.Println()
	fmt.Println("Creates:")
	fmt.Println("  <name>/scene.lgv    starter script (ball + text, 4s)")
	fmt.Println("  <name>/assets/       images and fonts go here")
	fmt.Println("  <name>/out/          rendered videos land here")
	fmt.Println()
	fmt.Println("Then:")
	fmt.Println("  kyfram render --out <name>/out/demo.mp4 <name>/scene.lgv")
}

// runNew scaffolds a starter project directory.
func runNew(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: kyfram new <name>")
		os.Exit(1)
	}
	name := args[0]
	if _, err := os.Stat(name); err == nil {
		fmt.Fprintf(os.Stderr, "%s: already exists\n", name)
		os.Exit(1)
	}
	dirs := []string{name, filepath.Join(name, "assets"), filepath.Join(name, "out")}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir %s: %v\n", d, err)
			os.Exit(1)
		}
	}
	scene := `scene(4, 30)

shape("bg", "rect", table{
  "0": table{x: 0, y: 0, w: 640, h: 360, color: "#101014"},
})

shape("ball", "circle", table{
  "0": table{x: 80, y: 300, r: 40, color: "#e64c40", ease: "easeOut"},
  "4": table{x: 560, y: 300, r: 40, color: "#40a3e6"},
})

shape("title", "text", table{
  "0": table{x: 320, y: 60, content: "my first kyfram scene", size: 34, color: "#ffffff", ease: "cubic-bezier(0.68, -0.55, 0.27, 1.55)"},
  "4": table{x: 320, y: 70, content: "my first kyfram scene", size: 34, color: "#ffffff"},
})
`
	if err := os.WriteFile(filepath.Join(name, "scene.lgv"), []byte(scene), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write scene.lgv: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("created %s/ — edit scene.lgv, then:\n", name)
	fmt.Printf("  kyfram render --out %s/out/demo.mp4 %s/scene.lgv\n", name, name)
}

// renderCommand builds "kyfram render [options] <script.lgv]".
// pflag accepts flags before or after the script, so the old
// flags-must-precede-the-script rule is gone.
func renderCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "render [options] <script.lgv>",
		Short: "Render a .lgv script to video or image",
		Run: func(cmd *cobra.Command, args []string) {
			runRender(cmd, args)
		},
	}
	cmd.Flags().String("out", "out.mp4", "output file path")
	cmd.Flags().Int("fps", 0, "override scene fps")
	cmd.Flags().Int("width", 0, "override canvas width")
	cmd.Flags().Int("height", 0, "override canvas height")
	cmd.Flags().String("font", "", "default typeface path")
	cmd.Flags().Int("crf", 23, "x264 CRF quality 0-51, lower is better (18 high, 28 small)")
	cmd.Flags().String("preset", "medium", "x264 preset: ultrafast,superfast,veryfast,faster,fast,medium,slow,slower,veryslow")
	cmd.Flags().String("format", "mp4", "output format: mp4 or png (single frame at t=0)")
	cmd.Flags().StringArray("var", nil, "template variable as key=value (repeatable)")
	return cmd
}

// viewCommand builds "kyfram view [options] <script.lgv>".
func viewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view [options] <script.lgv>",
		Short: "Live preview with scrub + hot reload",
		Run: func(cmd *cobra.Command, args []string) {
			runView(cmd, args)
		},
	}
	cmd.Flags().Int("port", 8080, "preview server port")
	cmd.Flags().String("font", "", "default typeface path")
	return cmd
}

// newCommand builds "kyfram new <name>".
func newCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "new <name>",
		Short: "Scaffold a starter project",
		Run: func(_ *cobra.Command, args []string) {
			runNew(args)
		},
	}
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
func runRender(cmd *cobra.Command, args []string) {
	flags := cmd.Flags()
	out, _ := flags.GetString("out")
	fps, _ := flags.GetInt("fps")
	width, _ := flags.GetInt("width")
	height, _ := flags.GetInt("height")
	font, _ := flags.GetString("font")
	crf, _ := flags.GetInt("crf")
	preset, _ := flags.GetString("preset")
	format, _ := flags.GetString("format")
	vars, _ := flags.GetStringArray("var")
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: kyfram render [--out f] [--fps n] [--width n] [--height n] [--font f] [--crf n] [--preset s] [--format mp4|png] [--var k=v ...] <script.lgv>")
		os.Exit(1)
	}
	for _, v := range []int{fps, width, height} {
		if v < 0 {
			fmt.Fprintln(os.Stderr, "flags: sizes must be positive")
			os.Exit(1)
		}
	}
	if crf < 0 || crf > 51 {
		fmt.Fprintln(os.Stderr, "crf must be 0-51")
		os.Exit(1)
	}
	validPreset := map[string]bool{"ultrafast": true, "superfast": true, "veryfast": true, "faster": true, "fast": true, "medium": true, "slow": true, "slower": true, "veryslow": true}
	if !validPreset[preset] {
		fmt.Fprintln(os.Stderr, "preset must be one of ultrafast,superfast,veryfast,faster,fast,medium,slow,slower,veryslow")
		os.Exit(1)
	}
	if format != "mp4" && format != "png" {
		fmt.Fprintln(os.Stderr, "format must be mp4 or png")
		os.Exit(1)
	}
	source, err := kyfram.LoadScript(args[0])
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
	if fps > 0 {
		tl.FPS = fps
	}
	if width > 0 {
		tl.Width = width
	}
	if height > 0 {
		tl.Height = height
	}
	if font != "" {
		tl.Font = font
	}
	if format == "png" && out == "out.mp4" {
		out = "out.png"
	}
	frames := int(tl.Duration*float64(tl.FPS) + 0.5)
	if frames < 1 {
		frames = 1
	}
	fmt.Fprintf(os.Stderr, "rendering %d frames (%dx%d @ %dfps, %.1fs) → %s\n",
		frames, tl.Width, tl.Height, tl.FPS, tl.Duration, out)
	if format == "png" {
		if err := kyfram.RenderFrame(tl, 0).SavePNG(out); err != nil {
			fmt.Fprintln(os.Stderr, "png:", err)
			os.Exit(1)
		}
		fmt.Println("ok:", out)
		return
	}
	if err := kyfram.EncodeWithOpts(tl, out, kyfram.EncodeOpts{CRF: crf, Preset: preset}); err != nil {
		fmt.Fprintln(os.Stderr, "encode:", err)
		os.Exit(1)
	}
	fmt.Println("ok:", out)
}
