package kyfram

import (
	"fmt"
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
