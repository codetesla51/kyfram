package kyfram

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/fogleman/gg"
)

const (
	frameW = 640
	frameH = 360

	// Only font proven in step 1; configurable fonts arrive with the
	// text vocabulary (step 8). CLI flags own the size later (step 10).
	frameFont = "/usr/share/fonts/noto/NotoSans-Regular.ttf"
)

// RenderFrame paints every entity at time t onto one canvas: resolve
// values through Interpolate, then dispatch on Kind. Unknown kinds are
// skipped so one experimental entity can't kill the whole frame.
func RenderFrame(tl Timeline, t float64) *gg.Context {
	w, h := tl.size()
	dc := gg.NewContext(w, h)
	dc.SetRGB(0.1, 0.1, 0.12)
	dc.Clear()

	for _, e := range tl.Entities {
		props := Interpolate(e.Keyframes, t)
		switch e.Kind {
		case "circle":
			dc.SetHexColor(propStr(props, "color", "#ffffff"))
			dc.DrawCircle(
				propNum(props, "x", float64(w)/2),
				propNum(props, "y", float64(h)/2),
				propNum(props, "r", 50),
			)
			dc.Fill()
		case "text":
			if err := dc.LoadFontFace(propStr(props, "font", frameFont), propNum(props, "size", 28)); err != nil {
				continue
			}
			dc.SetHexColor(propStr(props, "color", "#ffffff"))
			dc.DrawStringAnchored(
				propStr(props, "content", ""),
				propNum(props, "x", float64(w)/2),
				propNum(props, "y", float64(h)/2),
				0.5, 0.5,
			)
		case "rect":
			// x,y is the top-left corner; w,h default to a 100px square.
			dc.SetHexColor(propStr(props, "color", "#ffffff"))
			dc.DrawRectangle(
				propNum(props, "x", float64(w)/2-50),
				propNum(props, "y", float64(h)/2-50),
				propNum(props, "w", 100),
				propNum(props, "h", 100),
			)
			dc.Fill()
		case "line":
			dc.SetHexColor(propStr(props, "color", "#ffffff"))
			dc.SetLineWidth(propNum(props, "width", 2))
			dc.DrawLine(
				propNum(props, "x1", 0),
				propNum(props, "y1", float64(h)/2),
				propNum(props, "x2", float64(w)),
				propNum(props, "y2", float64(h)/2),
			)
			dc.Stroke()
		case "image":
			// ponytail: decoded from disk every frame; cache by path if slow.
			path := propStr(props, "path", "")
			if path == "" {
				continue
			}
			im, err := gg.LoadImage(path)
			if err != nil {
				continue
			}
			dc.DrawImageAnchored(im, int(propNum(props, "x", float64(w)/2)), int(propNum(props, "y", float64(h)/2)), 0.5, 0.5)
		}
	}
	return dc
}

// RenderSequence renders every frame of tl into dir as frame_%04d.png:
// frame count is Duration*FPS, frame i sits at t=i/FPS. Zero-padded
// names keep alphabetical order equal to time order. Failures name the
// frame index so a bad keyframe is findable without re-running blind.
func RenderSequence(tl Timeline, dir string) error {
	if tl.FPS <= 0 {
		return fmt.Errorf("fps %d: must be positive", tl.FPS)
	}
	n := frameCount(tl)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	for i := range n {
		if err := RenderFrame(tl, float64(i)/float64(tl.FPS)).SavePNG(filepath.Join(dir, fmt.Sprintf("frame_%04d.png", i))); err != nil {
			return fmt.Errorf("frame %d: %w", i, err)
		}
	}
	return nil
}

// frameCount converts Duration*FPS to a frame total (minimum 1), so a
// zero-length timeline still yields one still instead of nothing.
func frameCount(tl Timeline) int {
	n := int(tl.Duration*float64(tl.FPS) + 0.5)
	if n < 1 {
		n = 1
	}
	return n
}

// Encode renders tl and pipes raw RGBA frames to ffmpeg's stdin, which
// encodes out.mp4. Pixel format, size, and fps must match the flags
// exactly or the output corrupts — so failures report the exact command
// plus ffmpeg's own stderr instead of a bare exit code.
func Encode(tl Timeline, outPath string) error {
	if tl.FPS <= 0 {
		return fmt.Errorf("fps %d: must be positive", tl.FPS)
	}
	w, h := tl.size()
	args := []string{
		"-y",
		"-f", "rawvideo", "-pix_fmt", "rgba",
		"-s", fmt.Sprintf("%dx%d", w, h),
		"-r", strconv.Itoa(tl.FPS),
		"-i", "-",
	}
	// itsoffset shifts an input forward, so a beep lands on a visual
	// beat instead of the file start. Missing files fail inside ffmpeg
	// with its stderr attached below — no pre-check needed here.
	for _, a := range tl.Audio {
		if a.Offset != 0 {
			args = append(args, "-itsoffset", strconv.FormatFloat(a.Offset, 'f', 3, 64))
		}
		args = append(args, "-i", a.Path)
	}
	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p")
	switch len(tl.Audio) {
	case 0:
	case 1:
		// apad stretches short audio to the video end; -shortest then
		// caps exactly at the last frame instead of the last sample.
		args = append(args, "-af", "apad", "-c:a", "aac", "-shortest")
	default:
		args = append(args,
			"-filter_complex", fmt.Sprintf("amix=inputs=%d,apad", len(tl.Audio)),
			"-c:a", "aac", "-shortest")
	}
	args = append(args, outPath)
	cmd := exec.Command("ffmpeg", args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("ffmpeg stdin: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("ffmpeg start (%s): %w", cmd.String(), err)
	}
	for i := range frameCount(tl) {
		if _, err := stdin.Write(rgbaBytes(RenderFrame(tl, float64(i)/float64(tl.FPS)).Image())); err != nil {
			stdin.Close()
			waitErr := cmd.Wait()
			return fmt.Errorf("frame %d: %w (ffmpeg: %v\n%s)", i, err, waitErr, stderr.String())
		}
	}
	stdin.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("ffmpeg (%s): %w\n%s", cmd.String(), err, stderr.String())
	}
	return nil
}

// rgbaBytes exposes the canvas pixels for the ffmpeg pipe. gg canvases
// are *image.RGBA already; anything else gets converted rather than
// misread as raw bytes with the wrong stride.
func rgbaBytes(img image.Image) []byte {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba.Pix
	}
	dst := image.NewRGBA(img.Bounds())
	draw.Draw(dst, img.Bounds(), img, image.Point{}, draw.Src)
	return dst.Pix
}

// propNum reads a numeric prop with a default, so entities missing a
// field still draw instead of collapsing to zero.
func propNum(props map[string]any, key string, def float64) float64 {
	if v, ok := props[key]; ok {
		if f, ok := toFloat(v); ok {
			return f
		}
	}
	return def
}

// propStr reads a string prop with a default, so a missing color never
// hands SetHexColor an empty string.
func propStr(props map[string]any, key, def string) string {
	if v, ok := props[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}
