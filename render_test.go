package kyfram

import (
	"fmt"
	"image"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fogleman/gg"
)

// Hand-built Timeline must render: right size, circle paint lands at
// the interpolated position, text doesn't crash the frame.
func TestRenderFrame(t *testing.T) {
	tl := Timeline{
		Duration: 2,
		FPS:      30,
		Entities: []Entity{
			{
				Name: "ball",
				Kind: "circle",
				Keyframes: []Keyframe{
					{Time: 0, Props: map[string]any{"x": 100.0, "y": 160.0, "r": 80.0, "color": "#e64c40"}},
					{Time: 2, Props: map[string]any{"x": 320.0, "y": 160.0, "r": 80.0, "color": "#e64c40"}},
				},
			},
			{
				Name: "caption",
				Kind: "text",
				Keyframes: []Keyframe{
					{Time: 0, Props: map[string]any{"content": "hello kyfram", "x": 320.0, "y": 300.0, "size": 28.0, "color": "#ffffff"}},
					{Time: 2, Props: map[string]any{"content": "hello kyfram", "x": 320.0, "y": 300.0, "size": 28.0, "color": "#ffffff"}},
				},
			},
			{Name: "future", Kind: "starship", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{}},
			}},
		},
	}

	dc := RenderFrame(tl, 2)
	img := dc.Image()
	if img.Bounds().Dx() != frameW || img.Bounds().Dy() != frameH {
		t.Fatalf("size = %v, want %dx%d", img.Bounds(), frameW, frameH)
	}

	// At t=2 the ball sits at x=320, so the center pixel is ball paint.
	r, g, b, _ := img.At(320, 160).RGBA()
	if r>>8 != 0xe6 || g>>8 != 0x4c || b>>8 != 0x40 {
		t.Fatalf("center = #%02x%02x%02x, want #e64c40", r>>8, g>>8, b>>8)
	}
}

// Two frames at 10fps over 0.2s must land as frame_0000.png and
// frame_0001.png, non-empty, so a flip-through shows the motion.
func TestRenderSequence(t *testing.T) {
	tl := Timeline{
		Duration: 0.2,
		FPS:      10,
		Entities: []Entity{
			{
				Name: "ball",
				Kind: "circle",
				Keyframes: []Keyframe{
					{Time: 0, Props: map[string]any{"x": 100.0}},
					{Time: 0.2, Props: map[string]any{"x": 200.0}},
				},
			},
		},
	}

	dir := t.TempDir()
	if err := RenderSequence(tl, dir); err != nil {
		t.Fatalf("RenderSequence: %v", err)
	}
	for _, name := range []string{"frame_0000.png", "frame_0001.png"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if fi.Size() == 0 {
			t.Fatalf("%s is empty", name)
		}
	}
}

// Rect, line, and image kinds must all paint at their declared spots;
// a missing image path sketches nothing instead of crashing.
func TestRenderVocabulary(t *testing.T) {
	asset := filepath.Join(t.TempDir(), "dot.png")
	dot := gg.NewContext(40, 40)
	dot.SetHexColor("#ffffff")
	dot.Clear()
	if err := dot.SavePNG(asset); err != nil {
		t.Fatalf("asset: %v", err)
	}

	tl := Timeline{
		Duration: 1,
		FPS:      10,
		Entities: []Entity{
			{Name: "box", Kind: "rect", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"x": 100.0, "y": 100.0, "w": 80.0, "h": 60.0, "color": "#ff0000"}},
			}},
			{Name: "rule", Kind: "line", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"x1": 0.0, "y1": 300.0, "x2": 640.0, "y2": 300.0, "width": 6.0, "color": "#ffffff"}},
			}},
			{Name: "pic", Kind: "image", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"path": asset, "x": 320.0, "y": 180.0}},
			}},
			{Name: "ghost", Kind: "image", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{}},
			}},
		},
	}

	img := RenderFrame(tl, 0).Image()
	for _, c := range []struct {
		name, want string
		x, y       int
	}{
		{"rect", "#ff0000", 140, 130},
		{"line", "#ffffff", 100, 300},
		{"image", "#ffffff", 320, 180},
	} {
		r, g, b, _ := img.At(c.x, c.y).RGBA()
		got := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
		if got != c.want {
			t.Fatalf("%s pixel = %s, want %s", c.name, got, c.want)
		}
	}
}

// Two frames at 10fps must encode to a non-empty out.mp4 (skipped
// where no ffmpeg binary exists).
func TestEncode(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	tl := Timeline{
		Duration: 0.2,
		FPS:      10,
		Entities: []Entity{
			{
				Name: "ball",
				Kind: "circle",
				Keyframes: []Keyframe{
					{Time: 0, Props: map[string]any{"x": 100.0}},
					{Time: 0.2, Props: map[string]any{"x": 200.0}},
				},
			},
		},
	}

	out := filepath.Join(t.TempDir(), "out.mp4")
	if err := Encode(tl, out); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	fi, err := os.Stat(out)
	if err != nil {
		t.Fatalf("stat out.mp4: %v", err)
	}
	if fi.Size() == 0 {
		t.Fatal("out.mp4 is empty")
	}
}

// A declared beep must come out as a real audio stream in the mp4.
func TestEncodeAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("no ffprobe")
	}
	dir := t.TempDir()
	beep := filepath.Join(dir, "beep.mp3")
	gen := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=0.2",
		"-c:a", "libmp3lame", beep)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("tone: %v %s", err, out)
	}

	tl := Timeline{
		Duration: 0.5,
		FPS:      10,
		Entities: []Entity{{Name: "b", Kind: "circle", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{}},
		}}},
		Audio: []AudioTrack{{Path: beep, Offset: 0.1}},
	}
	out := filepath.Join(dir, "out.mp4")
	if err := Encode(tl, out); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	probe := exec.Command("ffprobe", "-v", "error",
		"-show_entries", "stream=codec_type", "-of", "csv", out)
	streams, err := probe.Output()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	if !strings.Contains(string(streams), "audio") {
		t.Fatalf("no audio stream:\n%s", streams)
	}
}

// Alpha 0 must show the background through the entity; alpha 1 is solid.
func TestRenderAlpha(t *testing.T) {
	bg := RenderFrame(Timeline{Entities: []Entity{}}, 0).Image()
	br, bgc, bb, _ := bg.At(320, 180).RGBA()

	ghost := RenderFrame(Timeline{Entities: []Entity{
		{Name: "b", Kind: "circle", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{"x": 320.0, "y": 180.0, "r": 80.0, "color": "#ff0000", "alpha": 0.0}},
		}},
	}}, 0).Image()
	r, g, b, _ := ghost.At(320, 180).RGBA()
	if r != br || g != bgc || b != bb {
		t.Fatalf("alpha 0 pixel = #%02x%02x%02x, want background", r>>8, g>>8, b>>8)
	}

	solid := RenderFrame(Timeline{Entities: []Entity{
		{Name: "b", Kind: "circle", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{"x": 320.0, "y": 180.0, "r": 80.0, "color": "#ff0000", "alpha": 1.0}},
		}},
	}}, 0).Image()
	r, g, b, _ = solid.At(320, 180).RGBA()
	if r>>8 != 0xff || g>>8 != 0x00 || b>>8 != 0x00 {
		t.Fatalf("alpha 1 pixel = #%02x%02x%02x, want #ff0000", r>>8, g>>8, b>>8)
	}
}

// A 0..90 degree arc passes through the 45-degree point; outside the
// sweep stays background.
func TestRenderArc(t *testing.T) {
	tl := Timeline{Entities: []Entity{
		{Name: "a", Kind: "arc", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{
				"x": 320.0, "y": 180.0, "r": 80.0,
				"start": 0.0, "end": 90.0, "width": 12.0, "color": "#ffffff",
			}},
		}},
	}}
	img := RenderFrame(tl, 0).Image()
	r, g, b, _ := img.At(377, 237).RGBA()
	if r>>8 != 0xff || g>>8 != 0xff || b>>8 != 0xff {
		t.Fatalf("arc 45deg = #%02x%02x%02x, want #ffffff", r>>8, g>>8, b>>8)
	}
	r, g, b, _ = img.At(263, 123).RGBA()
	if r>>8 == 0xff && g>>8 == 0xff && b>>8 == 0xff {
		t.Fatal("arc 225deg painted, want background")
	}
}

// Higher layer paints on top; equal layers keep declaration order.
func TestRenderLayers(t *testing.T) {
	paint := func(top string) string {
		layers := map[string]float64{"red": 0, "blue": 0}
		layers[top] = 1
		tl := Timeline{Entities: []Entity{
			{Name: "a", Kind: "rect", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"x": 270.0, "y": 130.0, "w": 100.0, "h": 100.0, "color": "#ff0000", "layer": layers["red"]}},
			}},
			{Name: "b", Kind: "rect", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"x": 270.0, "y": 130.0, "w": 100.0, "h": 100.0, "color": "#0000ff", "layer": layers["blue"]}},
			}},
		}}
		r, g, b, _ := RenderFrame(tl, 0).Image().At(320, 180).RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	if got := paint("blue"); got != "#0000ff" {
		t.Fatalf("blue on top = %s, want #0000ff", got)
	}
	if got := paint("red"); got != "#ff0000" {
		t.Fatalf("red on top = %s, want #ff0000", got)
	}
}

// Named colors resolve at the default alpha too — paint() used to hand the
// raw name to gg's hex parser on the alpha>=1 fast path, drawing invisible.
func TestRenderNamedColor(t *testing.T) {
	paint := func(color string) string {
		tl := Timeline{Entities: []Entity{
			{Name: "bg", Kind: "rect", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"x": 0.0, "y": 0.0, "w": 640.0, "h": 360.0, "color": "#000000"}},
			}},
			{Name: "fg", Kind: "circle", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"x": 320.0, "y": 180.0, "r": 50.0, "color": color}},
			}},
		}}
		r, g, b, _ := RenderFrame(tl, 0).Image().At(320, 180).RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	if got := paint("white"); got != "#ffffff" {
		t.Fatalf("named white = %s, want #ffffff", got)
	}
	if got := paint("coral"); got != "#e64c40" {
		t.Fatalf("named coral = %s, want #e64c40", got)
	}
	if got := paint("#ffffff"); got != "#ffffff" {
		t.Fatalf("hex white = %s, want #ffffff", got)
	}
}

// Rotate 90 turns a horizontal bar vertical; scale 3 triples a radius.
func TestRenderSpin(t *testing.T) {
	bar := Timeline{Entities: []Entity{
		{Name: "b", Kind: "rect", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{"x": 270.0, "y": 170.0, "w": 100.0, "h": 20.0, "color": "#ffffff", "rotate": 90.0}},
		}},
	}}
	img := RenderFrame(bar, 0).Image()
	if r, _, _, _ := img.At(320, 140).RGBA(); r>>8 != 0xff {
		t.Fatal("rotated bar: pixel above center not painted")
	}
	if r, g, b, _ := img.At(360, 180).RGBA(); r>>8 == 0xff && g>>8 == 0xff && b>>8 == 0xff {
		t.Fatal("rotated bar: pixel right of center painted, want background")
	}

	dot := Timeline{Entities: []Entity{
		{Name: "d", Kind: "circle", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{"x": 320.0, "y": 180.0, "r": 10.0, "color": "#ffffff", "scale": 3.0}},
		}},
	}}
	if r, _, _, _ := RenderFrame(dot, 0).Image().At(345, 180).RGBA(); r>>8 != 0xff {
		t.Fatal("scaled circle: radius-25 pixel not painted")
	}
}

// A looped 0.2s beep must still sound at the end of a 1s video.
func TestEncodeLoop(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("no ffmpeg")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("no ffprobe")
	}
	dir := t.TempDir()
	beep := filepath.Join(dir, "beep.mp3")
	gen := exec.Command("ffmpeg", "-y", "-v", "error",
		"-f", "lavfi", "-i", "sine=frequency=880:duration=0.2",
		"-c:a", "libmp3lame", beep)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("tone: %v %s", err, out)
	}

	tl := Timeline{
		Duration: 1,
		FPS:      10,
		Entities: []Entity{{Name: "b", Kind: "circle", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{}},
		}}},
		Audio: []AudioTrack{{Path: beep, Loop: true}},
	}
	out := filepath.Join(dir, "out.mp4")
	if err := Encode(tl, out); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	probe := exec.Command("ffprobe", "-v", "error",
		"-select_streams", "a", "-show_entries", "stream=duration",
		"-of", "default=noprint_wrappers=1:nokey=1", out)
	dur, err := probe.Output()
	if err != nil {
		t.Fatalf("probe: %v", err)
	}
	var seconds float64
	if _, err := fmt.Sscanf(string(dur), "%f", &seconds); err != nil {
		t.Fatalf("parse %q: %v", dur, err)
	}
	if seconds < 0.9 {
		t.Fatalf("looped audio duration = %vs, want ~1s", seconds)
	}
}

// A Timeline font flows into text rendering: bogus leaves background,
// valid paints glyphs.
func TestRenderFontOverride(t *testing.T) {
	mk := func(font string) int {
		tl := Timeline{Font: font, Entities: []Entity{
			{Name: "t", Kind: "text", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"content": "hello", "x": 320.0, "y": 180.0, "size": 64.0, "color": "#ffffff"}},
			}},
		}}
		img := RenderFrame(tl, 0).Image()
		n := 0
		for y := 100; y < 260; y += 4 {
			for x := 200; x < 440; x += 4 {
				if r, _, _, _ := img.At(x, y).RGBA(); r>>8 > 30 {
					n++
				}
			}
		}
		return n
	}
	if n := mk("/nope-missing.ttf"); n != 0 {
		t.Fatalf("bogus font painted %d pixels", n)
	}
	if n := mk("examples/assets/fonts/ArchivoBlack-Regular.ttf"); n < 100 {
		t.Fatalf("override font painted %d pixels, want a real word", n)
	}
}

// A one-cycle wave peaks and troughs where the sine says; zero
// amplitude is a straight line.
func TestRenderWave(t *testing.T) {
	mk := func(amp float64) image.Image {
		return RenderFrame(Timeline{Entities: []Entity{
			{Name: "w", Kind: "wave", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{
					"x": 220.0, "y": 180.0, "width": 200.0,
					"amplitude": amp, "frequency": 1.0, "phase": 0.0,
					"thickness": 8.0, "color": "#ffffff",
				}},
			}},
		}}, 0).Image()
	}
	at := func(img image.Image, x, y int) string {
		r, g, b, _ := img.At(x, y).RGBA()
		return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
	}
	img := mk(40)
	if got := at(img, 270, 220); got != "#ffffff" {
		t.Fatalf("wave peak = %s, want #ffffff", got)
	}
	if got := at(img, 370, 140); got != "#ffffff" {
		t.Fatalf("wave trough = %s, want #ffffff", got)
	}
	flat := mk(0)
	if got := at(flat, 320, 180); got != "#ffffff" {
		t.Fatalf("flat wave = %s, want #ffffff", got)
	}
	if got := at(flat, 320, 140); got == "#ffffff" {
		t.Fatal("flat wave painted off-line")
	}
}

// A triangle path fills its interior; a bad list draws nothing.
func TestRenderPath(t *testing.T) {
	tl := Timeline{Entities: []Entity{
		{Name: "tri", Kind: "path", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{
				"points": []any{270.0, 230.0, 370.0, 230.0, 320.0, 130.0},
				"filled": true, "color": "#ffffff",
			}},
		}},
	}}
	img := RenderFrame(tl, 0).Image()
	if r, _, _, _ := img.At(320, 190).RGBA(); r>>8 != 0xff {
		t.Fatal("triangle interior not filled")
	}

	bad := Timeline{Entities: []Entity{
		{Name: "x", Kind: "path", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{"points": []any{1.0}}},
		}},
	}}
	r, g, b, _ := RenderFrame(bad, 0).Image().At(320, 180).RGBA()
	if r>>8 != 0x19 || g>>8 != 0x19 {
		t.Fatalf("bad path painted #%02x%02x%02x", r>>8, g>>8, b>>8)
	}
}

// A horizontal black→white gradient is dark left, bright right.
func TestRenderGradient(t *testing.T) {
	tl := Timeline{Entities: []Entity{
		{Name: "g", Kind: "rect", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{
				"x": 270.0, "y": 130.0, "w": 100.0, "h": 100.0,
				"gradFrom": "#000000", "gradTo": "#ffffff", "gradAngle": 0.0,
			}},
		}},
	}}
	img := RenderFrame(tl, 0).Image()
	lr, _, _, _ := img.At(275, 180).RGBA()
	rr, _, _, _ := img.At(365, 180).RGBA()
	if lr>>8 > 100 {
		t.Fatalf("gradient left = %d, want dark", lr>>8)
	}
	if rr>>8 < 150 {
		t.Fatalf("gradient right = %d, want bright", rr>>8)
	}
}

// Blend ops combine full-canvas rects exactly: multiply keeps red on
// white, screen keeps blue on black, add makes magenta from red+blue.
func TestRenderBlend(t *testing.T) {
	full := func(col, blend string, layer float64) Entity {
		return Entity{Name: "r", Kind: "rect", Keyframes: []Keyframe{
			{Time: 0, Props: map[string]any{
				"x": 0.0, "y": 0.0, "w": 640.0, "h": 360.0,
				"color": col, "blend": blend, "layer": layer,
			}},
		}}
	}
	cases := []struct {
		name      string
		base, top Entity
		want      string
	}{
		{"multiply", full("#ffffff", "", 0), full("#ff0000", "multiply", 1), "#ff0000"},
		{"screen", full("#000000", "", 0), full("#0000ff", "screen", 1), "#0000ff"},
		{"add", full("#0000ff", "", 0), full("#ff0000", "add", 1), "#ff00ff"},
		{"unknown falls back", full("#000000", "", 0), full("#00ff00", "bogus", 1), "#00ff00"},
	}
	for _, c := range cases {
		tl := Timeline{Entities: []Entity{c.base, c.top}}
		r, g, b, _ := RenderFrame(tl, 0).Image().At(320, 180).RGBA()
		if got := fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8); got != c.want {
			t.Fatalf("%s = %s, want %s", c.name, got, c.want)
		}
	}
}

// Panning the camera right moves the world left on screen.
func TestRenderCamera(t *testing.T) {
	mk := func(cam []Keyframe) image.Image {
		return RenderFrame(Timeline{
			Entities: []Entity{{Name: "b", Kind: "circle", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{"x": 320.0, "y": 180.0, "r": 40.0, "color": "#ffffff"}},
			}}},
			Camera: cam,
		}, 0).Image()
	}
	img := mk(nil)
	if r, _, _, _ := img.At(320, 180).RGBA(); r>>8 != 0xff {
		t.Fatal("locked camera: center not painted")
	}
	panned := mk([]Keyframe{{Time: 0, Props: map[string]any{"x": 420.0, "y": 180.0, "zoom": 1.0}}})
	if r, _, _, _ := panned.At(220, 180).RGBA(); r>>8 != 0xff {
		t.Fatal("panned camera: ball should sit 100px left")
	}
	if r, g, b, _ := panned.At(320, 180).RGBA(); r>>8 == 0xff && g>>8 == 0xff && b>>8 == 0xff {
		t.Fatal("panned camera: center should be background")
	}
	zoomed := mk([]Keyframe{{Time: 0, Props: map[string]any{"zoom": 2.0}}})
	if r, _, _, _ := zoomed.At(320, 110).RGBA(); r>>8 != 0xff {
		t.Fatal("zoom 2: ball edge should reach 80px out")
	}
}

// Glyphs ride the path: horizontal text paints its band, diagonal text
// reaches where horizontal never could.
func TestRenderTextOnPath(t *testing.T) {
	bright := func(img image.Image, x0, y0, x1, y1 int) int {
		n := 0
		for y := y0; y < y1; y += 2 {
			for x := x0; x < x1; x += 2 {
				if r, _, _, _ := img.At(x, y).RGBA(); r>>8 > 60 {
					n++
				}
			}
		}
		return n
	}
	mk := func(pts []any) image.Image {
		return RenderFrame(Timeline{Entities: []Entity{
			{Name: "t", Kind: "text", Keyframes: []Keyframe{
				{Time: 0, Props: map[string]any{
					"content": "oooo", "size": 28.0, "color": "#ffffff", "points": pts,
				}},
			}},
		}}, 0).Image()
	}
	flat := mk([]any{200.0, 180.0, 440.0, 180.0})
	if n := bright(flat, 200, 160, 320, 200); n < 50 {
		t.Fatalf("flat path text painted %d pixels, want a word", n)
	}
	diag := mk([]any{200.0, 260.0, 320.0, 180.0})
	if n := bright(diag, 230, 200, 280, 240); n < 20 {
		t.Fatalf("diagonal text reached %d pixels up-right, want the climb", n)
	}
	if n := bright(flat, 230, 200, 280, 240); n > 0 {
		t.Fatalf("flat text leaked %d pixels up-right", n)
	}
}

// wrap prop breaks long text into multiple lines.
func TestRenderTextWrap(t *testing.T) {
	bright := func(img image.Image, x0, y0, x1, y1 int) int {
		n := 0
		for y := y0; y < y1; y += 2 {
			for x := x0; x < x1; x += 2 {
				if r, _, _, _ := img.At(x, y).RGBA(); r>>8 > 60 {
					n++
				}
			}
		}
		return n
	}
	// long text without wrap: paint band is wide
	nowrap := RenderFrame(Timeline{
		Entities: []Entity{{
			Name: "t", Kind: "text",
			Keyframes: []Keyframe{{
				Time: 0, Props: map[string]any{"content": "hello world this is long text", "x": 320.0, "y": 180.0, "size": 24.0, "color": "#ffffff"},
			}},
		}},
	}, 0).Image()
	// long text with wrap: paint spreads vertically
	wrapped := RenderFrame(Timeline{
		Entities: []Entity{{
			Name: "t", Kind: "text",
			Keyframes: []Keyframe{{
				Time: 0, Props: map[string]any{"content": "hello world this is long text", "x": 320.0, "y": 180.0, "size": 24.0, "color": "#ffffff", "wrap": 200.0},
			}},
		}},
	}, 0).Image()

	nw := bright(nowrap, 200, 160, 440, 200)
	wp := bright(wrapped, 200, 160, 440, 200)
	// wrapped should have fewer bright pixels in the single-line band
	// (text broke into shorter lines)
	if wp >= nw {
		t.Fatalf("wrap should reduce single-line paint: nowrap=%d wrapped=%d", nw, wp)
	}
}

// frameCount uses math.Round, not +0.5 truncation.
func TestFrameCountRounding(t *testing.T) {
	// 1.5s @ 30fps = 45 frames exactly
	got := frameCount(Timeline{Duration: 1.5, FPS: 30})
	if got != 45 {
		t.Fatalf("1.5s@30fps: got %d, want 45", got)
	}
	// edge: tiny duration still gives 1 frame
	got = frameCount(Timeline{Duration: 0.001, FPS: 30})
	if got != 1 {
		t.Fatalf("tiny duration: got %d, want 1", got)
	}
}
