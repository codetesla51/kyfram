package kyfram

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/fogleman/gg"
	"golang.org/x/image/font/basicfont"
)

const (
	frameW = 640
	frameH = 360

	// defaultFont is the fallback typeface: a Timeline Font or text font
	// prop wins whenever set, so no script is stuck with this path.
	// Archivo Black is the product face; Noto stays as a system fallback
	// if the bundled file is missing at runtime.
	defaultFont = "examples/assets/fonts/ArchivoBlack-Regular.ttf"
)

// RenderFrame paints the frame at time t: find the active scene (or the
// single implicit scene when none were declared), crossfade across scene
// boundaries when a transition is set, and draw.
func RenderFrame(tl Timeline, t float64) *gg.Context {
	w, h := tl.size()
	dc := gg.NewContext(w, h)
	drawFrame(dc, tl, t)
	return dc
}

// drawFrame paints one frame into dc. It resets first so a render worker
// can reuse the same canvas for every frame: stale transforms, clip
// masks, dash settings, and paths must not leak from frame to frame.
func drawFrame(dc *gg.Context, tl Timeline, t float64) {
	w, h := tl.size()
	resetFrame(dc, tl)
	if len(tl.Scenes) == 0 {
		// single implicit scene: everything declared, its own camera
		renderScene(dc, tl, tl.Entities, tl.Groups, tl.Camera, t)
		return
	}
	idx, start := sceneAt(tl, t)
	if idx < 0 {
		return
	}
	sc := &tl.Scenes[idx]
	// transition window: previous scene frozen at its end, this scene
	// composited in by the chosen effect over its duration
	if idx > 0 && sc.Transition != "" && sc.Transition != "cut" {
		trDur := transitionDuration(normTransition(sc.Transition), sc.TransitionDur)
		if t >= start && t < start+trDur {
			prev := &tl.Scenes[idx-1]
			prevEnd := start // previous scene ends exactly where this one starts
			renderScene(dc, tl, sceneLists(tl, prev), sceneGroups(tl, prev), prev.Camera, prevEnd)
			layer := gg.NewContext(w, h)
			renderScene(layer, tl, sceneLists(tl, sc), sceneGroups(tl, sc), sc.Camera, t)
			applyTransition(dc.Image().(*image.RGBA), layer.Image().(*image.RGBA), sc.Transition, (t-start)/trDur, w, h)
			return
		}
	}
	renderScene(dc, tl, sceneLists(tl, sc), sceneGroups(tl, sc), sc.Camera, t)
}

// resetFrame restores the gg state drawEntity assumes it starts with.
// Colors and line settings are reset because a reused canvas otherwise
// inherits the previous frame's last entity.
func resetFrame(dc *gg.Context, tl Timeline) {
	dc.Identity()
	dc.ResetClip()
	dc.ClearPath()
	dc.SetDash()
	dc.SetDashOffset(0)
	dc.SetLineWidth(1)
	dc.SetLineCap(gg.LineCapRound)
	dc.SetLineJoin(gg.LineJoinRound)
	dc.SetFillRule(gg.FillRuleWinding)
	dc.SetFontFace(basicfont.Face7x13)
	if tl.Transparent {
		dc.SetRGBA(0, 0, 0, 0)
	} else {
		dc.SetRGB(0.1, 0.1, 0.12)
	}
	dc.Clear()
}

// sceneLists merges global (pre-scene) declarations with a scene's own
// entities, so e.g. a watermark declared before scene() draws everywhere.
func sceneLists(tl Timeline, sc *Scene) []Entity {
	es := make([]Entity, 0, len(tl.Entities)+len(sc.Entities))
	es = append(es, tl.Entities...)
	es = append(es, sc.Entities...)
	return es
}

// sceneGroups mirrors sceneLists for groups.
func sceneGroups(tl Timeline, sc *Scene) []Group {
	gs := make([]Group, 0, len(tl.Groups)+len(sc.Groups))
	gs = append(gs, tl.Groups...)
	gs = append(gs, sc.Groups...)
	return gs
}

// sceneStartAt returns the absolute time scene i begins.
func sceneStartAt(tl Timeline, i int) float64 {
	var s float64
	for j := 0; j < i && j < len(tl.Scenes); j++ {
		s += tl.Scenes[j].Duration
	}
	return s
}

// sceneAt returns the scene covering time t and its start. Past the
// end, the last scene's start is returned (interpolation holds values).
func sceneAt(tl Timeline, t float64) (int, float64) {
	for i := range tl.Scenes {
		start := sceneStartAt(tl, i)
		if t < start+tl.Scenes[i].Duration {
			return i, start
		}
	}
	last := len(tl.Scenes) - 1
	return last, sceneStartAt(tl, last)
}

// renderScene draws one scene's entities/groups onto dc under cam,
// using this timeline's global groups as fallback. Shared by the
// single-scene path, per-scene dispatch, and offscreen crossfade layers.
func renderScene(dc *gg.Context, tl Timeline, entities []Entity, groups []Group, cam []Keyframe, t float64) {
	w, h := tl.size()
	// Camera reframes the world (see frameCamera); the background clear
	// stays put while entities travel.
	frameCamera(dc, cam, w, h, t)

	// Group transforms and hierarchy. A group's keyframes may name a
	// parent; the child's transform is applied inside the parent.
	groupProps := make(map[string]map[string]any, len(groups))
	groupLayer := make(map[string]float64, len(groups))
	groupParent := make(map[string]string, len(groups))
	for _, g := range groups {
		props := Interpolate(g.Keyframes, t)
		groupProps[g.Name] = props
		groupLayer[g.Name] = propNum(props, "layer", 0)
		// Parent is structural, not animated: keep the first declared
		// parent even if later keyframes omit the prop.
		groupParent[g.Name] = groupParentOf(g)
	}
	childGroups := make(map[string][]string, len(groups))
	var rootGroups []string
	for _, g := range groups {
		parent := groupParent[g.Name]
		if parent != "" && groupProps[parent] != nil {
			childGroups[parent] = append(childGroups[parent], g.Name)
		} else {
			rootGroups = append(rootGroups, g.Name)
		}
	}
	for parent := range childGroups {
		sort.SliceStable(childGroups[parent], func(i, j int) bool {
			return groupLayer[childGroups[parent][i]] < groupLayer[childGroups[parent][j]]
		})
	}
	sort.SliceStable(rootGroups, func(i, j int) bool {
		return groupLayer[rootGroups[i]] < groupLayer[rootGroups[j]]
	})

	// Partition entities into root and grouped.
	type framed struct {
		kind  string
		props map[string]any
		layer float64
		group string
	}
	var root []framed
	grouped := make(map[string][]framed)
	for _, e := range entities {
		props := Interpolate(e.Keyframes, t)
		f := framed{e.Kind, props, propNum(props, "layer", 0), e.Group}
		if e.Group != "" {
			if _, ok := groupProps[e.Group]; ok {
				grouped[e.Group] = append(grouped[e.Group], f)
				continue
			}
		}
		root = append(root, f)
	}
	sort.SliceStable(root, func(i, j int) bool { return root[i].layer < root[j].layer })
	for g := range grouped {
		sort.SliceStable(grouped[g], func(i, j int) bool { return grouped[g][i].layer < grouped[g][j].layer })
	}

	type groupXform struct {
		x, y   float64
		rotate float64
		scale  float64
		clip   map[string]any
	}
	type drawItem struct {
		isGroup bool
		layer   float64
		name    string
		framed  framed
	}

	drawFramed := func(f framed, chain []groupXform, alpha float64, inGroup bool) {
		props := copyProps(f.props)
		if alpha < 1 {
			props["alpha"] = propNum(props, "alpha", 1) * alpha
		}
		if inGroup {
			groupChildDefaults(f.kind, props)
		}
		mode := ""
		if m, ok := props["blend"].(string); ok && blendModes[m] {
			mode = m
		}
		if mode != "" || hasEffects(props) {
			layer := gg.NewContext(w, h)
			frameCamera(layer, cam, w, h, t)
			layer.Push()
			for _, xf := range chain {
				layer.Translate(xf.x, xf.y)
				layer.Rotate(gg.Radians(xf.rotate))
				layer.Scale(xf.scale, xf.scale)
				if hasClip(xf.clip) {
					applyClip(layer, xf.clip)
				}
			}
			drawEntity(layer, w, h, f.kind, props, defaultFace(tl))
			layer.Pop()
			img := layer.Image().(*image.RGBA)
			if hasEffects(props) {
				img = applyEffects(img, props)
			}
			dst := dc.Image().(*image.RGBA)
			if mode != "" {
				composite(dst, img, mode)
			} else {
				draw.Draw(dst, dst.Bounds(), img, image.Point{}, draw.Over)
			}
			return
		}
		drawEntity(dc, w, h, f.kind, props, defaultFace(tl))
	}

	var drawGroup func(name string, chain []groupXform, alpha float64)
	drawGroup = func(name string, chain []groupXform, alpha float64) {
		props := groupProps[name]
		if props == nil || len(chain) > len(groups) {
			return
		}
		root := len(chain) == 0
		gx, gy := 0.0, 0.0
		if root {
			gx, gy = float64(w)/2, float64(h)/2
		}
		gx = propNum(props, "x", gx)
		gy = propNum(props, "y", gy)
		gRot := propNum(props, "rotate", 0)
		gScale := propNum(props, "scale", 1)
		gt := groupXform{x: gx, y: gy, rotate: gRot, scale: gScale, clip: props}
		next := append(append([]groupXform{}, chain...), gt)
		cumAlpha := alpha * propNum(props, "alpha", 1)

		dc.Push()
		dc.Translate(gx, gy)
		dc.Rotate(gg.Radians(gRot))
		dc.Scale(gScale, gScale)
		if hasClip(props) {
			applyClip(dc, props)
		}

		var items []drawItem
		for _, f := range grouped[name] {
			items = append(items, drawItem{layer: f.layer, framed: f})
		}
		for _, child := range childGroups[name] {
			items = append(items, drawItem{isGroup: true, layer: groupLayer[child], name: child})
		}
		sort.SliceStable(items, func(i, j int) bool { return items[i].layer < items[j].layer })
		for _, it := range items {
			if it.isGroup {
				drawGroup(it.name, next, cumAlpha)
			} else {
				drawFramed(it.framed, next, cumAlpha, true)
			}
		}
		dc.Pop()
	}

	var order []drawItem
	for _, f := range root {
		order = append(order, drawItem{layer: f.layer, framed: f})
	}
	for _, name := range rootGroups {
		order = append(order, drawItem{isGroup: true, layer: groupLayer[name], name: name})
	}
	sort.SliceStable(order, func(i, j int) bool { return order[i].layer < order[j].layer })
	for _, it := range order {
		if it.isGroup {
			drawGroup(it.name, nil, 1)
		} else {
			drawFramed(it.framed, nil, 1, false)
		}
	}
	dc.Pop()
}

// groupParentOf reads the structural parent from the group's keyframes.
// Parent is not a tweened property, so a later keyframe that omits it
// must not re-root the subtree.
func groupParentOf(g Group) string {
	for _, kf := range g.Keyframes {
		if s, ok := kf.Props["parent"].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// groupChildDefaults gives group children local coordinates instead of
// the canvas-center defaults used by root entities.
func groupChildDefaults(kind string, props map[string]any) {
	switch kind {
	case "circle", "star", "ellipse", "text", "svg":
		if _, ok := props["x"]; !ok {
			props["x"] = 0.0
		}
		if _, ok := props["y"]; !ok {
			props["y"] = 0.0
		}
	case "rect", "roundRect":
		if _, ok := props["x"]; !ok {
			props["x"] = -propNum(props, "w", 100) / 2
		}
		if _, ok := props["y"]; !ok {
			props["y"] = -propNum(props, "h", 100) / 2
		}
	}
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
		if !setFace(dc, propStr(props, "font", face), propNum(props, "size", 28)) {
			return
		}
		spin(dc, cx, cy, props, func() {
			paint(dc, props)
			content := propStr(props, "content", "")
			// A points list turns the line into a path the glyphs ride.
			if pts, ok := props["points"].([]any); ok {
				drawTextOnPath(dc, content, pts)
				return
			}
			wrapW := propNum(props, "wrap", 0)
			if wrapW > 0 || strings.ContainsAny(content, "\r\n") {
				drawWrappedText(dc, content, cx, cy, wrapW, propNum(props, "lineHeight", 1.2))
			} else {
				dc.DrawStringAnchored(content, cx, cy, 0.5, 0.5)
			}
		})
	case "rect":
		// x,y is the top-left corner; w,h default to a 100px square.
		// rotate pivots at the rect center, not at x,y.
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
			from, to := trimWindow(props)
			if to > from {
				ax, ay := lerpPoint(x1, y1, x2, y2, from)
				bx, by := lerpPoint(x1, y1, x2, y2, to)
				dc.DrawLine(ax, ay, bx, by)
			}
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
	case "svg":
		// A static SVG file becomes one entity. x,y is the center, w/h the
		// target box; the document's viewBox and preserveAspectRatio handle
		// fitting. Colors, groups, transforms, and basic gradients are kept,
		// while text/scripts/filters are ignored by the importer.
		path := propStr(props, "path", "")
		if path == "" {
			return
		}
		doc, err := loadSVG(path)
		if err != nil || doc == nil {
			return
		}
		dw := propNum(props, "w", doc.Width)
		dh := propNum(props, "h", doc.Height)
		if dw <= 0 {
			dw = doc.ViewBox[2]
		}
		if dh <= 0 {
			dh = doc.ViewBox[3]
		}
		if dw <= 0 || dh <= 0 {
			return
		}
		spin(dc, cx, cy, props, func() {
			drawSVGDoc(dc, doc, cx-dw/2, cy-dh/2, dw, dh, propNum(props, "alpha", 1))
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
		// closed when filled/closed is set. Same-length lists morph (see
		// blend); mismatched ones snap like strings. Cap/join via cap/join props.
		pcx, pcy, ok := pathCenter(props)
		if !ok {
			pcx, pcy = cx, cy
		}
		spin(dc, pcx, pcy, props, func() {
			paint(dc, props)
			if d, ok := props["d"].(string); ok && d != "" {
				if contours, err := flattenPathD(d); err == nil {
					drawPathContours(dc, props, contours)
				}
				return
			}
			drawPath(dc, props)
		})
	case "polygon":
		pcx, pcy, ok := pathCenter(props)
		if !ok {
			pcx, pcy = cx, cy
		}
		// gradient bbox from points if available
		x0, y0, x1, y1 := pcx-50, pcy-50, pcx+50, pcy+50
		if pts, ok := props["points"].([]any); ok && len(pts) >= 4 {
			if bx0, by0, bx1, by1, ok := pointsBBox(pts); ok {
				x0, y0, x1, y1 = bx0, by0, bx1, by1
			}
		}
		spin(dc, pcx, pcy, props, func() {
			if !gradientFill(dc, x0, y0, x1, y1, props) {
				paint(dc, props)
			}
			drawPolygon(dc, props)
		})
	case "roundRect":
		// x,y is the top-left corner; rotate pivots at the rect center.
		rx, ry := propNum(props, "x", float64(w)/2-50), propNum(props, "y", float64(h)/2-50)
		rw, rh := propNum(props, "w", 100), propNum(props, "h", 100)
		radius := propNum(props, "radius", 10)
		spin(dc, rx+rw/2, ry+rh/2, props, func() {
			if !gradientFill(dc, rx, ry, rx+rw, ry+rh, props) {
				paint(dc, props)
			}
			dc.DrawRoundedRectangle(rx, ry, rw, rh, radius)
			dc.Fill()
		})
	case "star":
		// star: x,y (center) r (outer), points (int), inner (0-1), color
		spin(dc, cx, cy, props, func() {
			paint(dc, props)
			drawStar(dc, props, cx, cy)
		})
	case "ellipse":
		// ellipse: x,y center, rx,ry radii (if ry missing, use rx)
		rx := propNum(props, "rx", propNum(props, "r", 50))
		ry := propNum(props, "ry", rx)
		spin(dc, cx, cy, props, func() {
			if !gradientFill(dc, cx-rx, cy-ry, cx+rx, cy+ry, props) {
				paint(dc, props)
			}
			dc.DrawEllipse(cx, cy, rx, ry)
			dc.Fill()
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
	step := 1.0
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

// drawWrappedText lays out explicit newlines and optionally wraps each
// paragraph to wrapW pixels, centering the block vertically around cy.
// A horizontal wrap value of zero means "only break on newlines".
func drawWrappedText(dc *gg.Context, content string, cx, cy, wrapW, lineHeight float64) {
	lines := textLines(dc, content, wrapW)
	// estimate line height from current font size (gg has no FontMetrics)
	lineH := 0.0
	if _, h := dc.MeasureString("Mj"); h > 0 {
		lineH = h
	} else {
		lineH = 28 // fallback
	}
	if lineHeight <= 0 {
		lineHeight = 1.2
	}
	lineH *= lineHeight
	blockH := lineH * float64(len(lines))
	y := cy - blockH/2 + lineH/2
	for _, line := range lines {
		dc.DrawStringAnchored(line, cx, y, 0.5, 0.5)
		y += lineH
	}
}

// textLines splits on explicit newlines first, then greedily breaks each
// paragraph on spaces when maxW > 0. Blank lines and single words longer
// than maxW are kept rather than silently dropped.
func textLines(dc *gg.Context, s string, maxW float64) []string {
	var lines []string
	for _, para := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if maxW <= 0 || strings.TrimSpace(para) == "" {
			lines = append(lines, para)
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := words[0]
		for _, w := range words[1:] {
			try := cur + " " + w
			tw, _ := dc.MeasureString(try)
			if tw > maxW && cur != "" {
				lines = append(lines, cur)
				cur = w
			} else {
				cur = try
			}
		}
		lines = append(lines, cur)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

// requested, passing straight through otherwise so the common path
// costs nothing. Rotation needs no per-kind math: the canvas moves,
// the shape code stays naive.
func spin(dc *gg.Context, cx, cy float64, props map[string]any, draw func()) {
	rot := propNum(props, "rotate", 0)
	sc := propNum(props, "scale", 1)
	clip := hasClip(props)
	if rot == 0 && sc == 1 && !clip {
		draw()
		return
	}
	dc.Push()
	dc.Translate(cx, cy)
	dc.Rotate(gg.Radians(rot))
	dc.Scale(sc, sc)
	dc.Translate(-cx, -cy)
	if clip {
		applyClip(dc, props)
	}
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
	n := int(math.Round(totalDuration(tl) * float64(tl.FPS)))
	if n < 1 {
		n = 1
	}
	return n
}

// totalDuration resolves how long the video runs: the sum of scenes
// when scenes were declared, else the single Duration.
func totalDuration(tl Timeline) float64 {
	if len(tl.Scenes) > 0 {
		var d float64
		for _, s := range tl.Scenes {
			d += s.Duration
		}
		return d
	}
	return tl.Duration
}

// EncodeOpts controls x264 quality. CRF 0-51 lower is better (18 high, 23 default, 28 small),
// Preset is speed/quality tradeoff: ultrafast..veryslow (medium default).
type EncodeOpts struct {
	CRF    int
	Preset string
	// Workers is the number of goroutines painting frames. Zero or
	// negative means runtime.NumCPU(). Each worker owns one canvas.
	Workers int
	// Progress reports piped frames as (done, total), from the writer
	// goroutine. done counts frames handed to ffmpeg in order, so the
	// final call is always (total, total) — percent is done*100/total.
	Progress func(done, total int)
}

// Encode renders tl and pipes raw RGBA frames to ffmpeg's stdin, which
// encodes out.mp4. Pixel format, size, and fps must match the flags
// exactly or the output corrupts — so failures report the exact command
// plus ffmpeg's own stderr instead of a bare exit code.
func Encode(tl Timeline, outPath string) error {
	return EncodeWithOpts(tl, outPath, EncodeOpts{CRF: 23, Preset: "medium"})
}

// EncodeWithOpts is Encode with explicit CRF/preset. Frames are painted
// by opts.Workers goroutines, then written to ffmpeg in frame order.
func EncodeWithOpts(tl Timeline, outPath string, opts EncodeOpts) error {
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
	crf := opts.CRF
	if crf < 0 {
		crf = 23
	}
	if crf > 51 {
		crf = 51
	}
	preset := opts.Preset
	if preset == "" {
		preset = "medium"
	}
	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p", "-crf", strconv.Itoa(crf), "-preset", preset)
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
	n := frameCount(tl)
	err = encodeFrames(tl, opts.Workers, func(i int, buf []byte) error {
		if _, err := stdin.Write(buf); err != nil {
			return err
		}
		if opts.Progress != nil {
			opts.Progress(i+1, n)
		}
		return nil
	})
	// ffmpeg only finishes after stdin closes, on both success and
	// failure. Waiting here also lets us attach its stderr to our error.
	stdin.Close()
	waitErr := cmd.Wait()
	if err != nil {
		if waitErr != nil {
			return fmt.Errorf("%w (ffmpeg: %v\n%s)", err, waitErr, stderr.String())
		}
		return fmt.Errorf("%w\n%s", err, stderr.String())
	}
	if waitErr != nil {
		return fmt.Errorf("ffmpeg (%s): %w\n%s", cmd.String(), waitErr, stderr.String())
	}
	return nil
}

// frameResult is one painted frame waiting to be written. Buffers are
// owned by the writer once received.
type frameResult struct {
	index int
	buf   []byte
}

// encodeFrames paints every frame with workers goroutines and calls write
// in strict frame order. write must not retain buf past its return.
// Unwritten frames are capped at twice the worker count, so a slow ffmpeg
// cannot make fast painters pile up memory.
func encodeFrames(tl Timeline, workers int, write func(int, []byte) error) error {
	n := frameCount(tl)
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	if workers > n {
		workers = n
	}
	if workers < 1 {
		workers = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		errMu    sync.Mutex
		firstErr error
	)
	fail := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
		cancel()
	}

	jobs := make(chan int)
	results := make(chan frameResult, workers)
	slots := make(chan struct{}, workers*2)

	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer wg.Done()
			if err := frameWorker(ctx, tl, jobs, results, slots); err != nil {
				fail(err)
			}
		}()
	}

	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		if err := writeFramesInOrder(ctx, n, results, slots, write); err != nil {
			fail(err)
		}
	}()

feed:
	for i := 0; i < n; i++ {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)

	wg.Wait()
	<-writerDone

	errMu.Lock()
	err := firstErr
	errMu.Unlock()
	return err
}

// frameWorker paints jobs forever with one canvas. Draw state is reset at
// the start of each frame by drawFrame, and the completed pixels are
// copied because the canvas is cleared for the next frame.
func frameWorker(ctx context.Context, tl Timeline, jobs <-chan int, results chan<- frameResult, slots chan struct{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("render frame: %v", r)
		}
	}()
	dc := gg.NewContext(tl.size())
	for {
		select {
		case <-ctx.Done():
			return nil
		case idx, ok := <-jobs:
			if !ok {
				return nil
			}
			// Hold a slot while the frame is in flight, not just while
			// queued, so memory stays bounded when ffmpeg falls behind.
			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				return nil
			}
			drawFrame(dc, tl, float64(idx)/float64(tl.FPS))
			buf := copyFramePixels(dc.Image())
			select {
			case results <- frameResult{index: idx, buf: buf}:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

// writeFramesInOrder holds early arrivals in a map until their turn, then
// writes 0, 1, 2... and releases each frame's memory slot.
func writeFramesInOrder(ctx context.Context, n int, results <-chan frameResult, slots chan struct{}, write func(int, []byte) error) error {
	pending := make(map[int][]byte)
	for next := 0; next < n; next++ {
		for {
			if buf, ok := pending[next]; ok {
				delete(pending, next)
				if err := write(next, buf); err != nil {
					return fmt.Errorf("frame %d: %w", next, err)
				}
				<-slots
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case res := <-results:
				pending[res.index] = res.buf
			}
		}
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

// copyFramePixels returns a fresh RGBA buffer for the ffmpeg pipe. The
// copy is mandatory: the worker reuses and clears its canvas for the next
// frame, so handing out dc.Image().(*image.RGBA).Pix would be overwritten.
func copyFramePixels(img image.Image) []byte {
	rgba, ok := img.(*image.RGBA)
	if !ok {
		b := img.Bounds()
		dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
		draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
		return dst.Pix
	}
	b := rgba.Bounds()
	w := b.Dx()
	row := w * 4
	if rgba.Stride == row {
		start := rgba.PixOffset(b.Min.X, b.Min.Y)
		out := make([]byte, row*b.Dy())
		copy(out, rgba.Pix[start:start+row*b.Dy()])
		return out
	}
	out := make([]byte, 0, row*b.Dy())
	for y := b.Min.Y; y < b.Max.Y; y++ {
		start := rgba.PixOffset(b.Min.X, y)
		out = append(out, rgba.Pix[start:start+row]...)
	}
	return out
}

// paint sets the entity color with the alpha prop applied, so fades are
// real transparency instead of bg-color matching (which ghosts over other
// entities). Images ignore alpha: gg has no per-draw opacity for them.
func paint(dc *gg.Context, props map[string]any) {
	col := propStr(props, "color", "#ffffff")
	a := propNum(props, "alpha", 1)
	if a > 1 {
		a = 1
	}
	if a < 0 {
		a = 0
	}
	// resolve names here, not just on the faded path: gg's SetHexColor
	// only reads #rrggbb, so a raw "white" used to draw invisible/black
	// at the default alpha. validate.go already guarantees parseable.
	c, err := parseColor(col)
	if err != nil {
		c = [3]float64{255, 255, 255}
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
// Cap/join via cap: "round"|"butt"|"square", join: "round"|"bevel"|"miter".
// Closed via closed or filled (filled implies closed).
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
	closed := propBool(props, "closed", propBool(props, "filled", false))
	drawPathContours(dc, props, []pathContour{{points: pts, closed: closed}})
}

// drawPathContours draws one or more flattened subpaths with the same
// fill/stroke settings. Each contour owns its own MoveTo/ClosePath, so
// an SVG d string can contain multiple M...Z runs.
func drawPathContours(dc *gg.Context, props map[string]any, contours []pathContour) {
	if len(contours) == 0 {
		return
	}
	if capStr, ok := props["cap"].(string); ok {
		switch capStr {
		case "round":
			dc.SetLineCap(gg.LineCapRound)
		case "square":
			dc.SetLineCap(gg.LineCapSquare)
		default:
			dc.SetLineCap(gg.LineCapButt)
		}
	}
	if joinStr, ok := props["join"].(string); ok {
		switch joinStr {
		case "round":
			dc.SetLineJoin(gg.LineJoinRound)
		case "bevel", "miter":
			dc.SetLineJoin(gg.LineJoinBevel)
		}
	}
	closeAll := propBool(props, "closed", propBool(props, "filled", false))
	if propBool(props, "filled", false) {
		traceContours(dc, contours, closeAll)
		dc.Fill()
		return
	}
	from, to := trimWindow(props)
	if from <= 0 && to >= 1 {
		traceContours(dc, contours, closeAll)
	} else {
		drawTrimmedContours(dc, contours, closeAll, from, to)
	}
	dc.SetLineWidth(propNum(props, "width", 3))
	dc.Stroke()
}

// traceContours feeds every subpath to the context without changing
// fill/stroke settings.
func traceContours(dc *gg.Context, contours []pathContour, closeAll bool) {
	for _, c := range contours {
		if len(c.points) < 4 {
			continue
		}
		dc.MoveTo(c.points[0], c.points[1])
		for i := 2; i+1 < len(c.points); i += 2 {
			dc.LineTo(c.points[i], c.points[i+1])
		}
		if closeAll || c.closed {
			dc.ClosePath()
		}
	}
}

// trimWindow reads the normalized visible range. trim=1 and trimStart=0
// (the defaults) draw the full path.
func trimWindow(props map[string]any) (float64, float64) {
	from := clamp01(propNum(props, "trimStart", 0))
	to := clamp01(propNum(props, "trim", 1))
	if to < from {
		return 0, 0
	}
	return from, to
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func lerpPoint(x0, y0, x1, y1, f float64) (float64, float64) {
	return x0 + (x1-x0)*f, y0 + (y1-y0)*f
}

// drawTrimmedContours strokes the arc-length slice [from,to] of every
// subpath. It walks the already-flattened contours, so d strings and
// point paths share the same draw-on behavior.
func drawTrimmedContours(dc *gg.Context, contours []pathContour, closeAll bool, from, to float64) {
	total := 0.0
	for _, c := range contours {
		total += contourLength(c, closeAll || c.closed)
	}
	if total <= 0 {
		return
	}
	startDist, endDist := from*total, to*total
	if endDist <= startDist {
		return
	}
	dist := 0.0
	for _, c := range contours {
		n := len(c.points) / 2
		if n < 2 {
			continue
		}
		closed := closeAll || c.closed
		segs := n - 1
		if closed {
			segs++
		}
		drawing := false
		for i := 0; i < segs; i++ {
			x0, y0 := c.points[i*2], c.points[i*2+1]
			j := (i + 1) % n
			x1, y1 := c.points[j*2], c.points[j*2+1]
			segLen := math.Hypot(x1-x0, y1-y0)
			if segLen <= 1e-9 {
				continue
			}
			segStart, segEnd := dist, dist+segLen
			if segEnd > startDist && segStart < endDist {
				a := math.Max(startDist, segStart)
				b := math.Min(endDist, segEnd)
				ax, ay := lerpPoint(x0, y0, x1, y1, (a-segStart)/segLen)
				bx, by := lerpPoint(x0, y0, x1, y1, (b-segStart)/segLen)
				if !drawing {
					dc.MoveTo(ax, ay)
					drawing = true
				} else {
					dc.LineTo(ax, ay)
				}
				dc.LineTo(bx, by)
				if b < segEnd {
					return
				}
			}
			dist = segEnd
			if dist >= endDist {
				return
			}
		}
	}
}

func contourLength(c pathContour, closed bool) float64 {
	n := len(c.points) / 2
	if n < 2 {
		return 0
	}
	total := 0.0
	segs := n - 1
	if closed {
		segs++
	}
	for i := 0; i < segs; i++ {
		j := (i + 1) % n
		total += math.Hypot(c.points[j*2]-c.points[i*2], c.points[j*2+1]-c.points[i*2+1])
	}
	return total
}

// drawPolygon fills or strokes a closed polygon from points. Same morph as path.
func drawPolygon(dc *gg.Context, props map[string]any) {
	raw, ok := props["points"].([]any)
	if !ok || len(raw) < 6 || len(raw)%2 != 0 {
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
	dc.ClosePath()
	if propBool(props, "filled", true) {
		dc.Fill()
		return
	}
	dc.SetLineWidth(propNum(props, "width", 2))
	dc.Stroke()
}

// drawStar draws a star centered at cx,cy with outer radius r, points count, inner ratio.
func drawStar(dc *gg.Context, props map[string]any, cx, cy float64) {
	r := propNum(props, "r", 50)
	if r <= 0 {
		return
	}
	points := int(propNum(props, "points", 5))
	if points < 2 {
		points = 5
	}
	inner := propNum(props, "inner", 0.45)
	if inner < 0 {
		inner = 0
	}
	if inner > 1 {
		inner = 1
	}
	outer := r
	innerR := r * inner
	for i := 0; i < points*2; i++ {
		ang := math.Pi*float64(i)/float64(points) - math.Pi/2
		rad := outer
		if i%2 == 1 {
			rad = innerR
		}
		x := cx + math.Cos(ang)*rad
		y := cy + math.Sin(ang)*rad
		if i == 0 {
			dc.MoveTo(x, y)
		} else {
			dc.LineTo(x, y)
		}
	}
	dc.ClosePath()
	if propBool(props, "filled", true) {
		dc.Fill()
	} else {
		dc.SetLineWidth(propNum(props, "width", 2))
		dc.Stroke()
	}
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

// pointsBBox returns bbox of a flat points list.
func pointsBBox(raw []any) (float64, float64, float64, float64, bool) {
	if len(raw) < 4 || len(raw)%2 != 0 {
		return 0, 0, 0, 0, false
	}
	minX, maxX, minY, maxY := 0.0, 0.0, 0.0, 0.0
	for i := 0; i < len(raw); i += 2 {
		x, ok1 := toFloat(raw[i])
		y, ok2 := toFloat(raw[i+1])
		if !ok1 || !ok2 {
			return 0, 0, 0, 0, false
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
	return minX, minY, maxX, maxY, true
}

// pathCenter is the bbox middle of a point list for spin transforms;
// invalid lists report false and callers fall back to canvas center.
func pathCenter(props map[string]any) (float64, float64, bool) {
	if d, ok := props["d"].(string); ok && d != "" {
		if contours, err := flattenPathD(d); err == nil {
			if minX, minY, maxX, maxY, ok := pathDBounds(contours); ok {
				return (minX + maxX) / 2, (minY + maxY) / 2, true
			}
		}
		return 0, 0, false
	}
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

// compositeFade alpha-blends src over dst by factor f (0..1) in the
// source-over sense: where src has painted pixels they appear at
// intensity f over the dst content — a crossfade. Unpainted src
// pixels leave dst untouched.
func compositeFade(dst, src *image.RGBA, f float64) {
	for i := 0; i+3 < len(dst.Pix) && i+3 < len(src.Pix); i += 4 {
		sa := float64(src.Pix[i+3]) / 255
		w := sa * f
		if w == 0 {
			continue
		}
		da := float64(dst.Pix[i+3]) / 255
		oa := w + da*(1-w)
		if oa == 0 {
			continue
		}
		dst.Pix[i] = uint8((float64(src.Pix[i])*w + float64(dst.Pix[i])*da*(1-w)) / oa)
		dst.Pix[i+1] = uint8((float64(src.Pix[i+1])*w + float64(dst.Pix[i+1])*da*(1-w)) / oa)
		dst.Pix[i+2] = uint8((float64(src.Pix[i+2])*w + float64(dst.Pix[i+2])*da*(1-w)) / oa)
		dst.Pix[i+3] = uint8(oa*255 + 0.5)
	}
}

// frameCamera pushes the world-to-screen transform: the x/y world point
// lands on the screen center, scaled by zoom. Empty keys interpolate to
// nothing, so the transform is identity and costs one matrix. The caller
// pops after drawing (or discards the context for offscreen layers).
func frameCamera(dc *gg.Context, cam []Keyframe, w, h int, t float64) {
	dc.Push()
	props := Interpolate(cam, t)
	zoom := propNum(props, "zoom", 1)
	if zoom <= 0 {
		zoom = 1
	}
	dc.Translate(float64(w)/2, float64(h)/2)
	dc.Scale(zoom, zoom)
	dc.Translate(-propNum(props, "x", float64(w)/2), -propNum(props, "y", float64(h)/2))
}

// drawTextOnPath lays runes along a flat [x1,y1,...] polyline, each
// rotated to its segment, glyph centers walking arc-length from the
// path start. Bad point lists and empty strings draw nothing; runes
// past the path end are dropped, not wrapped.
func drawTextOnPath(dc *gg.Context, s string, raw []any) {
	pts := make([]float64, 0, len(raw))
	for _, v := range raw {
		f, ok := toFloat(v)
		if !ok {
			return
		}
		pts = append(pts, f)
	}
	if len(pts) < 4 || len(pts)%2 != 0 || s == "" {
		return
	}
	type seg struct{ x, y, ang, len float64 }
	segs := make([]seg, 0, len(pts)/2-1)
	for i := 0; i+3 < len(pts); i += 2 {
		dx, dy := pts[i+2]-pts[i], pts[i+3]-pts[i+1]
		segs = append(segs, seg{pts[i], pts[i+1], math.Atan2(dy, dx), math.Hypot(dx, dy)})
	}
	si := 0
	used := 0.0
	at := func(dist float64) (float64, float64, float64, bool) {
		for si < len(segs) && used+segs[si].len < dist {
			used += segs[si].len
			si++
		}
		if si >= len(segs) {
			return 0, 0, 0, false
		}
		g := segs[si]
		d := dist - used
		return g.x + math.Cos(g.ang)*d, g.y + math.Sin(g.ang)*d, g.ang, true
	}
	cursor := 0.0
	for _, r := range s {
		ch := string(r)
		w, _ := dc.MeasureString(ch)
		px, py, ang, ok := at(cursor + w/2)
		if !ok {
			return
		}
		dc.Push()
		dc.Translate(px, py)
		dc.Rotate(ang)
		dc.DrawStringAnchored(ch, 0, 0, 0.5, 0.5)
		dc.Pop()
		cursor += w
	}
}
