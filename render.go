package kyfram

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
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

	// defaultFont is the fallback typeface: a Timeline Font or text font
	// prop wins whenever set, so no script is stuck with this path.
	defaultFont = "/usr/share/fonts/noto/NotoSans-Regular.ttf"
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
		// Blend modes composite offscreen then merge pixelwise;
		// everything else paints straight onto the canvas.
		if mode, ok := f.props["blend"].(string); ok && blendModes[mode] {
			layer := gg.NewContext(w, h)
			drawEntity(layer, w, h, f.kind, f.props, defaultFace(tl))
			composite(dc.Image().(*image.RGBA), layer.Image().(*image.RGBA), mode)
			continue
		}
		drawEntity(dc, w, h, f.kind, f.props, defaultFace(tl))
	}
	return dc
}

// defaultFace resolves the fallback typeface: explicit Timeline font
// wins, otherwise the built-in default.
func defaultFace(tl Timeline) string {
	if tl.Font != "" {
		return tl.Font
	}
	return defaultFont
}

// drawEntity paints one entity. Unknown kinds and unloadable assets
// return silently so one experimental entity can't kill the frame.
func drawEntity(dc *gg.Context, w, h int, kind string, props map[string]any, face string) {
	cx := propNum(props, "x", float64(w)/2)
	cy := propNum(props, "y", float64(h)/2)
	switch kind {
	case "circle":
		spin(dc, cx, cy, props, func() {
			r := propNum(props, "r", 50)
			if !gradientFill(dc, cx-r, cy-r, cx+r, cy+r, props) {
				paint(dc, props)
			}
			dc.DrawCircle(cx, cy, r)
			dc.Fill()
		})
	case "text":
		if err := dc.LoadFontFace(propStr(props, "font", face), propNum(props, "size", 28)); err != nil {
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
			if !gradientFill(dc, rx, ry, rx+rw, ry+rh, props) {
				paint(dc, props)
			}
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
	case "wave":
		// The engine owns the sine: scripts declare endpoints and
		// shaping numbers, all keyframeable, so waves morph like any
		// other prop. Frequency is cycles across the width, phase is
		// degrees. Zero amplitude is a straight line, for free.
		spin(dc, cx, cy, props, func() {
			paint(dc, props)
			dc.SetLineWidth(propNum(props, "thickness", 3))
			drawWave(dc, props, float64(w))
			dc.Stroke()
		})
	case "path":
		// Flat point list [x1,y1,x2,y2...]: stroke it, or fill it
		// closed when filled is set. Same-length lists morph (see
		// blend); mismatched ones snap like strings.
		pcx, pcy, ok := pathCenter(props)
		if !ok {
			pcx, pcy = cx, cy
		}
		spin(dc, pcx, pcy, props, func() {
			paint(dc, props)
			drawPath(dc, props)
		})
	}
}

// drawWave strokes y = amp*sin(2π(freq*s/width + phase/360)) from x to
// x+width, sampled every few pixels. Negative widths draw backwards —
// the math doesn't care about direction.
func drawWave(dc *gg.Context, props map[string]any, canvasW float64) {
	x := propNum(props, "x", 0)
	y := propNum(props, "y", 180)
	width := propNum(props, "width", canvasW)
	if width == 0 {
		return
	}
	amp := propNum(props, "amplitude", 30)
	freq := propNum(props, "frequency", 1)
	cyc := propNum(props, "phase", 0) / 360
	step := 3.0
	if width < 0 {
		step = -step
	}
	first := true
	for s := 0.0; ; s += step {
		done := (step > 0 && s >= width) || (step < 0 && s <= width)
		if done {
			s = width
		}
		px := x + s
		py := y + amp*math.Sin(2*math.Pi*(freq*s/width+cyc))
		if first {
			dc.MoveTo(px, py)
			first = false
		} else {
			dc.LineTo(px, py)
		}
		if done {
			break
		}
	}
}

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

// drawPath strokes (or closed-fills) a flat [x1,y1,...] point list.
// Short or non-numeric lists draw nothing instead of half a shape.
func drawPath(dc *gg.Context, props map[string]any) {
	raw, ok := props["points"].([]any)
	if !ok || len(raw) < 4 || len(raw)%2 != 0 {
		return
	}
	pts := make([]float64, len(raw))
	for i, v := range raw {
		f, ok := toFloat(v)
		if !ok {
			return
		}
		pts[i] = f
	}
	dc.MoveTo(pts[0], pts[1])
	for i := 2; i < len(pts); i += 2 {
		dc.LineTo(pts[i], pts[i+1])
	}
	if propBool(props, "filled", false) {
		dc.ClosePath()
		dc.Fill()
		return
	}
	dc.SetLineWidth(propNum(props, "width", 3))
	dc.Stroke()
}

// propBool reads a boolean prop with a default; anything non-boolean
// falls back so a typo can't arm a flag by accident.
func propBool(props map[string]any, key string, def bool) bool {
	if v, ok := props[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

// pathCenter is the bbox middle of a point list for spin transforms;
// invalid lists report false and callers fall back to canvas center.
func pathCenter(props map[string]any) (float64, float64, bool) {
	raw, ok := props["points"].([]any)
	if !ok || len(raw) < 4 || len(raw)%2 != 0 {
		return 0, 0, false
	}
	minX, maxX := 0.0, 0.0
	minY, maxY := 0.0, 0.0
	for i := 0; i < len(raw); i += 2 {
		x, ok := toFloat(raw[i])
		if !ok {
			return 0, 0, false
		}
		y, ok := toFloat(raw[i+1])
		if !ok {
			return 0, 0, false
		}
		if i == 0 || x < minX {
			minX = x
		}
		if i == 0 || x > maxX {
			maxX = x
		}
		if i == 0 || y < minY {
			minY = y
		}
		if i == 0 || y > maxY {
			maxY = y
		}
	}
	return (minX + maxX) / 2, (minY + maxY) / 2, true
}

// gradientFill paints bbox with a linear gradFrom→gradTo gradient at
// gradAngle degrees, honoring alpha. It reports whether a gradient was
// declared so callers fall back to flat paint otherwise. Fill kinds
// only (circle, rect) — strokes keep their flat color.
func gradientFill(dc *gg.Context, x0, y0, x1, y1 float64, props map[string]any) bool {
	from, ok := props["gradFrom"].(string)
	if !ok {
		return false
	}
	to, ok := props["gradTo"].(string)
	if !ok {
		return false
	}
	fc, err := parseColor(from)
	if err != nil {
		return false
	}
	tc, err := parseColor(to)
	if err != nil {
		return false
	}
	a := propNum(props, "alpha", 1)
	if a > 1 {
		a = 1
	}
	if a < 0 {
		a = 0
	}
	ang := gg.Radians(propNum(props, "gradAngle", 0))
	cx, cy := (x0+x1)/2, (y0+y1)/2
	dx, dy := math.Cos(ang), math.Sin(ang)
	r := math.Hypot(x1-x0, y1-y0) / 2
	g := gg.NewLinearGradient(cx-dx*r, cy-dy*r, cx+dx*r, cy+dy*r)
	stop := func(c [3]float64) color.NRGBA {
		return color.NRGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), uint8(a*255 + 0.5)}
	}
	g.AddColorStop(0, stop(fc))
	g.AddColorStop(1, stop(tc))
	dc.SetFillStyle(g)
	return true
}

// blendModes names the per-entity compositing ops. Unknown names fall
// back to normal painting where they're read.
var blendModes = map[string]bool{"multiply": true, "screen": true, "add": true}

// composite merges src onto dst pixelwise under one op. Bytes are read
// as straight RGBA (exact for opaque art, approximate on translucent
// edges) and transparent src pixels are skipped outright.
func composite(dst, src *image.RGBA, mode string) {
	mix := func(s, d float64) float64 {
		switch mode {
		case "multiply":
			return s * d
		case "screen":
			return 1 - (1-s)*(1-d)
		default: // add
			if s+d > 1 {
				return 1
			}
			return s + d
		}
	}
	for i := 0; i+3 < len(dst.Pix) && i+3 < len(src.Pix); i += 4 {
		sa := float64(src.Pix[i+3]) / 255
		if sa == 0 {
			continue
		}
		da := float64(dst.Pix[i+3]) / 255
		oa := sa + da*(1-sa)
		if oa == 0 {
			continue
		}
		sr, sg, sb := float64(src.Pix[i])/255, float64(src.Pix[i+1])/255, float64(src.Pix[i+2])/255
		dr, dg, db := float64(dst.Pix[i])/255, float64(dst.Pix[i+1])/255, float64(dst.Pix[i+2])/255
		dst.Pix[i] = uint8((mix(sr, dr)*sa + dr*da*(1-sa)) / oa * 255)
		dst.Pix[i+1] = uint8((mix(sg, dg)*sa + dg*da*(1-sa)) / oa * 255)
		dst.Pix[i+2] = uint8((mix(sb, db)*sa + db*da*(1-sa)) / oa * 255)
		dst.Pix[i+3] = uint8(oa*255 + 0.5)
	}
}
