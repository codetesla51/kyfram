package kyfram

import (
	"os"
	"path/filepath"
	"testing"
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
