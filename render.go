package kyfram

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"sync"

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

	// Layers are paint order: lower draws first. Stable sort keeps
	// declaration order within a layer, so old scripts behave the same.
	type framed struct {
		kind  string
		props map[string]any
		layer float64
	}
	framed_ := make([]framed, 0, len(tl.Entities))
	for _, e := range tl.Entities {
		props := Interpolate(e.Keyframes, t)
		framed_ = append(framed_, framed{e.Kind, props, propNum(props, "layer", 0)})
	}
	sort.SliceStable(framed_, func(i, j int) bool { return framed_[i].layer < framed_[j].layer })
	for _, f := range framed_ {
		drawEntity(dc, w, h, f.kind, f.props)
	}
	return dc
}

// drawEntity paints one entity. Unknown kinds and unloadable assets
// return silently so one experimental entity can't kill the frame.
func drawEntity(dc *gg.Context, w, h int, kind string, props map[string]any) {
	cx := propNum(props, "x", float64(w)/2)
	cy := propNum(props, "y", float64(h)/2)
	switch kind {
	case "circle":
		spin(dc, cx, cy, props, func() {
			paint(dc, props)
			dc.DrawCircle(cx, cy, propNum(props, "r", 50))
			dc.Fill()
		})
	case "text":
		if err := dc.LoadFontFace(propStr(props, "font", frameFont), propNum(props, "size", 28)); err != nil {
			return
		}
		spin(dc, cx, cy, props, func() {
			paint(dc, props)
			dc.DrawStringAnchored(propStr(props, "content", ""), cx, cy, 0.5, 0.5)
		})
	case "rect":
		// x,y is the top-left corner; w,h default to a 100px square.
		rx, ry := propNum(props, "x", float64(w)/2-50), propNum(props, "y", float64(h)/2-50)
		rw, rh := propNum(props, "w", 100), propNum(props, "h", 100)
		spin(dc, rx+rw/2, ry+rh/2, props, func() {
			paint(dc, props)
			dc.DrawRectangle(rx, ry, rw, rh)
			dc.Fill()
		})
	case "line":
		x1, y1 := propNum(props, "x1", 0), propNum(props, "y1", float64(h)/2)
		x2, y2 := propNum(props, "x2", float64(w)), propNum(props, "y2", float64(h)/2)
		spin(dc, (x1+x2)/2, (y1+y2)/2, props, func() {
			paint(dc, props)
			dc.SetLineWidth(propNum(props, "width", 2))
			dc.DrawLine(x1, y1, x2, y2)
			dc.Stroke()
		})
	case "arc":
		// Angles are degrees in scripts (gg wants radians); a full
		// 0..360 sweep is a ring, partial sweeps are loader hands.
		spin(dc, cx, cy, props, func() {
			paint(dc, props)
			dc.SetLineWidth(propNum(props, "width", 4))
			dc.DrawArc(cx, cy, propNum(props, "r", 50),
				gg.Radians(propNum(props, "start", 0)),
				gg.Radians(propNum(props, "end", 360)))
			dc.Stroke()
		})
	case "image":
		path := propStr(props, "path", "")
		if path == "" {
			return
		}
		im, err := loadImage(path)
		if err != nil {
			return
		}
		spin(dc, cx, cy, props, func() {
			dc.DrawImageAnchored(im, int(cx), int(cy), 0.5, 0.5)
		})
	}
}

// spin wraps drawing with centered rotation (degrees) and scale when
// requested, passing straight through otherwise so the common path
// costs nothing. Rotation needs no per-kind math: the canvas moves,
// the shape code stays naive.
func spin(dc *gg.Context, cx, cy float64, props map[string]any, draw func()) {
	rot := propNum(props, "rotate", 0)
	sc := propNum(props, "scale", 1)
	if rot == 0 && sc == 1 {
		draw()
		return
	}
	dc.Push()
	dc.Translate(cx, cy)
	dc.Rotate(gg.Radians(rot))
	dc.Scale(sc, sc)
	dc.Translate(-cx, -cy)
	draw()
	dc.Pop()
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
		// stream_loop repeats the input forever; apad+shortest below
		// still cap the mix at the last video frame.
		if a.Loop {
			args = append(args, "-stream_loop", "-1")
		}
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

// imgCache keeps decoded assets across frames: one PNG decoded once,
// not once per frame for the whole video. Process-lifetime; a render
// never mutates source files mid-run.
var imgCache = struct {
	sync.RWMutex
	m map[string]image.Image
}{m: make(map[string]image.Image)}

// loadImage reads an asset through the cache. Failures stay errors so
// the caller can skip the entity — a missing file is not fatal.
func loadImage(path string) (image.Image, error) {
	imgCache.RLock()
	im, ok := imgCache.m[path]
	imgCache.RUnlock()
	if ok {
		return im, nil
	}
	im, err := gg.LoadImage(path)
	if err != nil {
		return nil, err
	}
	imgCache.Lock()
	imgCache.m[path] = im
	imgCache.Unlock()
	return im, nil
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

// paint sets the entity color with the alpha prop applied, so fades are
// real transparency instead of bg-color matching (which ghosts over other
// entities). Images ignore alpha: gg has no per-draw opacity for them.
func paint(dc *gg.Context, props map[string]any) {
	col := propStr(props, "color", "#ffffff")
	a := propNum(props, "alpha", 1)
	if a >= 1 {
		dc.SetHexColor(col)
		return
	}
	if a < 0 {
		a = 0
	}
	c, err := parseColor(col)
	if err != nil {
		dc.SetHexColor(col)
		return
	}
	dc.SetRGBA(c[0]/255, c[1]/255, c[2]/255, a)
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
