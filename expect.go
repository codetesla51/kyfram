package kyfram

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// expectStep is the gap between geometry samples. A 0.1s grid catches
// motion that leaves the canvas between keyframes without making large
// scripts slow; a mismatch lands within 0.1s of the real crossing.
const expectStep = 0.1

// expectTolerance is how far a checked number may drift. Half a pixel
// absorbs float math, interpolation rounding, and hand-written 320 vs
// 320.0 without hiding a real layout mistake.
const expectTolerance = 0.5

type expectKind int

const (
	expectValues expectKind = iota
	expectOnCanvas
	expectApart
)

// expectValue is one prop table at one absolute time.
type expectValue struct {
	time  float64
	props map[string]any
}

// expectation is one parsed expect() declaration.
type expectation struct {
	kind   expectKind
	names  []string
	values []expectValue
	// scene is the open scene index when declared, or -1 for a global
	// expectation. Names are resolved inside that scene first so scripts
	// can reuse "ball" in every scene without ambiguity.
	scene int
}

// parseExpectation converts the checks table into a typed check.
// offset is the open scene's start, so times written inside a scene are
// scene-local like every other declaration.
func parseExpectation(names []string, raw map[string]any, offset float64, scene int) (expectation, error) {
	exp := expectation{names: names, scene: scene}
	var values []expectValue
	var special []string
	for k, v := range raw {
		switch k {
		case "always", "apart":
			special = append(special, k)
		default:
			t, err := strconv.ParseFloat(k, 64)
			if err != nil {
				return exp, fmt.Errorf(`expect: unknown check %q (want a time like "1.0", "always", or "apart")`, k)
			}
			props, ok := v.(map[string]any)
			if !ok {
				return exp, fmt.Errorf("expect: check at %q must be a table of props", k)
			}
			for p, pv := range props {
				if !contains(knownProps, p) {
					return exp, fmt.Errorf("expect: unknown prop %q at %q%s", p, k, suggest(knownProps, p))
				}
				if contains(numProps, p) {
					if _, ok := toFloat(pv); !ok {
						return exp, fmt.Errorf("expect: prop %q at %q must be a number", p, k)
					}
				}
			}
			values = append(values, expectValue{time: t + offset, props: props})
		}
	}
	if len(special) > 1 {
		return exp, fmt.Errorf("expect: cannot combine always and apart")
	}
	if len(special) == 1 && len(values) > 0 {
		return exp, fmt.Errorf("expect: cannot mix time checks with %q", special[0])
	}
	switch {
	case len(special) == 1 && special[0] == "always":
		s, ok := raw["always"].(string)
		if !ok || s != "onCanvas" {
			return exp, fmt.Errorf(`expect: always must be "onCanvas"`)
		}
		exp.kind = expectOnCanvas
	case len(special) == 1:
		b, ok := raw["apart"].(bool)
		if !ok || !b {
			return exp, fmt.Errorf("expect: apart must be true")
		}
		exp.kind = expectApart
	default:
		exp.kind = expectValues
	}
	switch exp.kind {
	case expectApart:
		if len(names) != 2 {
			return exp, fmt.Errorf("expect: apart needs two entity names, got %d", len(names))
		}
	case expectOnCanvas, expectValues:
		if len(names) != 1 {
			return exp, fmt.Errorf("expect: this check needs one entity name, got %d", len(names))
		}
	}
	if exp.kind == expectValues && len(values) == 0 {
		return exp, fmt.Errorf("expect: no checks declared")
	}
	sort.Slice(values, func(i, j int) bool { return values[i].time < values[j].time })
	exp.values = values
	return exp, nil
}

// timelineView is one scene's slice of absolute time plus the entities
// and groups visible during it (globals included).
type timelineView struct {
	start, end float64
	entities   []Entity
	groups     []Group
	// globals is how many leading entities came from the timeline-wide
	// list; they are the same shapes in every scene, so canvas checks
	// aggregate them instead of judging each scene window separately.
	globals int
}

// timelineViews splits the timeline into scene windows. With no scenes
// the whole timeline is one window, which keeps hand-built Timelines and
// the single implicit scene working the same way.
func timelineViews(tl Timeline) []timelineView {
	if len(tl.Scenes) == 0 {
		return []timelineView{{start: 0, end: tl.Duration, entities: tl.Entities, groups: tl.Groups, globals: len(tl.Entities)}}
	}
	views := make([]timelineView, 0, len(tl.Scenes))
	start := 0.0
	for i := range tl.Scenes {
		sc := &tl.Scenes[i]
		views = append(views, timelineView{
			start:    start,
			end:      start + sc.Duration,
			entities: sceneLists(tl, sc),
			groups:   sceneGroups(tl, sc),
			globals:  len(tl.Entities),
		})
		start += sc.Duration
	}
	return views
}

// viewAt returns the window containing t; past the end it holds the last.
func viewAt(tl Timeline, t float64) timelineView {
	if len(tl.Scenes) == 0 {
		return timelineView{start: 0, end: tl.Duration, entities: tl.Entities, groups: tl.Groups, globals: len(tl.Entities)}
	}
	i, start := sceneAt(tl, t)
	sc := &tl.Scenes[i]
	return timelineView{start: start, end: start + sc.Duration, entities: sceneLists(tl, sc), groups: sceneGroups(tl, sc), globals: len(tl.Entities)}
}

// sampleTimes returns start, start+step, ... and always includes end.
// A non-multiple duration ends on an exact final sample, not a short one.
func sampleTimes(start, end, step float64) []float64 {
	if step <= 0 {
		step = expectStep
	}
	if end < start {
		end = start
	}
	times := make([]float64, 0, int((end-start)/step)+2)
	for i := 0; ; i++ {
		t := start + float64(i)*step
		if t > end-1e-9 {
			break
		}
		times = append(times, t)
	}
	if len(times) == 0 || math.Abs(times[len(times)-1]-end) > 1e-9 {
		times = append(times, end)
	}
	return times
}

// bbox is an axis-aligned world-space box.
type bbox struct {
	minX, minY, maxX, maxY float64
}

// overlapsCanvas reports whether any part of the box is visible.
func (b bbox) overlapsCanvas(w, h float64) bool {
	return b.maxX >= 0 && b.minX <= w && b.maxY >= 0 && b.minY <= h
}

// intersects reports whether two boxes share any area (edges count).
func (b bbox) intersects(o bbox) bool {
	return b.minX <= o.maxX && b.maxX >= o.minX && b.minY <= o.maxY && b.maxY >= o.minY
}

func (b bbox) String() string {
	return fmt.Sprintf("x=%g..%g y=%g..%g", b.minX, b.maxX, b.minY, b.maxY)
}

// placedEntity is an entity resolved at one time: props with renderer
// defaults filled, plus its group's translation and alpha.
type placedEntity struct {
	entity Entity
	props  map[string]any
	group  string
	gx, gy float64
	gAlpha float64
}

// placeEntity interpolates an entity and resolves its group offset.
// Group x/y default to canvas center, exactly like renderScene.
func placeEntity(tl Timeline, v timelineView, e Entity, t float64) placedEntity {
	w, h := tl.size()
	lp := Interpolate(e.Keyframes, t)
	p := placedEntity{entity: e, gAlpha: 1}
	p.props = resolveProps(e.Kind, lp, float64(w), float64(h), false)
	if e.Group == "" {
		return p
	}
	g, ok := findGroup(v.groups, e.Group)
	if !ok {
		return p
	}
	gp := Interpolate(g.Keyframes, t)
	p.group = e.Group
	p.gx = propNum(gp, "x", float64(w)/2)
	p.gy = propNum(gp, "y", float64(h)/2)
	p.gAlpha = propNum(gp, "alpha", 1)
	p.props = resolveProps(e.Kind, lp, float64(w), float64(h), true)
	return p
}

func findGroup(groups []Group, name string) (Group, bool) {
	for _, g := range groups {
		if g.Name == name {
			return g, true
		}
	}
	return Group{}, false
}

// resolveProps fills the same defaults drawEntity uses for the fields
// geometry checks read. inGroup switches position defaults to group-local
// coordinates (circle/star/ellipse/text at 0,0, rects at -w/2,-h/2), as
// the renderer does before translating by the group.
func resolveProps(kind string, raw map[string]any, w, h float64, inGroup bool) map[string]any {
	p := copyProps(raw)
	def := func(k string, v any) {
		if _, ok := p[k]; !ok {
			p[k] = v
		}
	}
	def("alpha", 1.0)
	switch kind {
	case "circle", "star":
		if inGroup {
			def("x", 0.0)
			def("y", 0.0)
		} else {
			def("x", w/2)
			def("y", h/2)
		}
		def("r", 50.0)
	case "ellipse":
		if inGroup {
			def("x", 0.0)
			def("y", 0.0)
		} else {
			def("x", w/2)
			def("y", h/2)
		}
		def("rx", propNum(p, "r", 50.0))
		def("ry", propNum(p, "rx", 50.0))
	case "text":
		if inGroup {
			def("x", 0.0)
			def("y", 0.0)
		} else {
			def("x", w/2)
			def("y", h/2)
		}
		def("size", 28.0)
	case "rect", "roundRect":
		def("w", 100.0)
		def("h", 100.0)
		if inGroup {
			def("x", -propNum(p, "w", 100)/2)
			def("y", -propNum(p, "h", 100)/2)
		} else {
			def("x", w/2-50)
			def("y", h/2-50)
		}
	case "line":
		def("x1", 0.0)
		def("y1", h/2)
		def("x2", w)
		def("y2", h/2)
	case "arc":
		def("x", w/2)
		def("y", h/2)
		def("r", 50.0)
	case "wave":
		def("x", 0.0)
		def("y", 180.0)
		def("width", w)
		def("amplitude", 30.0)
	case "image":
		def("x", w/2)
		def("y", h/2)
	}
	def("color", "#ffffff")
	return p
}

// bbox returns the entity's world-space box. Text without points and
// images are approximated from their content/metrics; everything else
// comes from the entity's own geometry. Rotation and group scale are not
// applied because canvas checks are deliberately axis-aligned.
func (p placedEntity) bbox(tl Timeline) (bbox, bool) {
	w, h := tl.size()
	b, ok := entityBBox(p.entity.Kind, p.props, float64(w), float64(h))
	if !ok {
		return bbox{}, false
	}
	b.minX += p.gx
	b.maxX += p.gx
	b.minY += p.gy
	b.maxY += p.gy
	return b, true
}

func entityBBox(kind string, p map[string]any, w, h float64) (bbox, bool) {
	cx := propNum(p, "x", w/2)
	cy := propNum(p, "y", h/2)
	switch kind {
	case "circle", "star":
		r := propNum(p, "r", 50)
		return bbox{cx - r, cy - r, cx + r, cy + r}, true
	case "ellipse":
		rx := propNum(p, "rx", propNum(p, "r", 50))
		ry := propNum(p, "ry", rx)
		return bbox{cx - rx, cy - ry, cx + rx, cy + ry}, true
	case "rect", "roundRect":
		rw, rh := propNum(p, "w", 100), propNum(p, "h", 100)
		return bbox{cx, cy, cx + rw, cy + rh}, true
	case "line":
		x1, y1 := propNum(p, "x1", 0), propNum(p, "y1", h/2)
		x2, y2 := propNum(p, "x2", w), propNum(p, "y2", h/2)
		return bbox{min(x1, x2), min(y1, y2), max(x1, x2), max(y1, y2)}, true
	case "arc":
		r := propNum(p, "r", 50)
		return bbox{cx - r, cy - r, cx + r, cy + r}, true
	case "wave":
		x, y := propNum(p, "x", 0), propNum(p, "y", 180)
		ww, amp := propNum(p, "width", w), propNum(p, "amplitude", 30)
		return bbox{min(x, x+ww), y - amp, max(x, x+ww), y + amp}, true
	case "path", "polygon":
		raw, ok := p["points"].([]any)
		if !ok {
			return bbox{}, false
		}
		return pointsBox(raw)
	case "text":
		if raw, ok := p["points"].([]any); ok && len(raw) >= 4 {
			return pointsBox(raw)
		}
		size := propNum(p, "size", 28)
		content := propStr(p, "content", "")
		width := float64(len([]rune(content))) * size * 0.6
		return bbox{cx - width/2, cy - size/2, cx + width/2, cy + size/2}, true
	case "image":
		path := propStr(p, "path", "")
		if path == "" {
			return bbox{cx, cy, cx, cy}, true
		}
		im, err := loadImage(path)
		if err != nil {
			return bbox{cx, cy, cx, cy}, true
		}
		b := im.Bounds()
		hw, hh := float64(b.Dx())/2, float64(b.Dy())/2
		return bbox{cx - hw, cy - hh, cx + hw, cy + hh}, true
	}
	return bbox{}, false
}

func pointsBox(raw []any) (bbox, bool) {
	x0, y0, x1, y1, ok := pointsBBox(raw)
	if !ok {
		return bbox{}, false
	}
	return bbox{x0, y0, x1, y1}, true
}

// value reads a checked prop. x/y are returned in world coordinates so
// an assertion on a group child reads the same position the viewer sees.
func (p placedEntity) value(key string) (any, bool) {
	v, ok := p.props[key]
	if !ok {
		return nil, false
	}
	if key == "x" || key == "y" {
		f, ok := toFloat(v)
		if !ok {
			return v, true
		}
		if key == "x" {
			return f + p.gx, true
		}
		return f + p.gy, true
	}
	return v, true
}

// checkExpectations runs every declared assertion before a frame is drawn.
func checkExpectations(tl Timeline, expects []expectation) error {
	if len(expects) == 0 {
		return nil
	}
	views := timelineViews(tl)
	for _, exp := range expects {
		var err error
		switch exp.kind {
		case expectValues:
			err = checkValues(tl, views, exp)
		case expectOnCanvas:
			err = checkAlwaysOnCanvas(tl, views, exp)
		case expectApart:
			err = checkApart(tl, views, exp)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func checkValues(tl Timeline, views []timelineView, exp expectation) error {
	name := exp.names[0]
	var view timelineView
	var ent Entity
	found := false
	for _, v := range scopedViews(views, exp) {
		if e, ok := viewEntity(v, name); ok {
			view, ent, found = v, e, true
			break
		}
	}
	if !found {
		return fmt.Errorf("expect: entity %q not found", name)
	}
	for _, ev := range exp.values {
		p := placeEntity(tl, view, ent, ev.time)
		keys := make([]string, 0, len(ev.props))
		for k := range ev.props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			got, ok := p.value(k)
			if ok && valueEqual(got, ev.props[k]) {
				continue
			}
			return fmt.Errorf("expect %s @%s: %s is %s, wanted %s",
				name, fmtTime(ev.time), k, fmtValue(got, ok), fmtValue(ev.props[k], true))
		}
	}
	return nil
}

func checkAlwaysOnCanvas(tl Timeline, views []timelineView, exp expectation) error {
	name := exp.names[0]
	found := false
	for _, v := range scopedViews(views, exp) {
		e, ok := viewEntity(v, name)
		if !ok {
			continue
		}
		found = true
		for _, t := range sampleTimes(v.start, v.end, expectStep) {
			if !entityOnCanvas(tl, v, e, t) {
				return fmt.Errorf("expect %s @%s: off canvas", name, fmtTime(t))
			}
		}
	}
	if !found {
		return fmt.Errorf("expect: entity %q not found", name)
	}
	return nil
}

func checkApart(tl Timeline, views []timelineView, exp expectation) error {
	aName, bName := exp.names[0], exp.names[1]
	foundA, foundB := false, false
	for _, v := range scopedViews(views, exp) {
		a, okA := viewEntity(v, aName)
		b, okB := viewEntity(v, bName)
		if okA {
			foundA = true
		}
		if okB {
			foundB = true
		}
		if !okA || !okB {
			continue
		}
		for _, t := range sampleTimes(v.start, v.end, expectStep) {
			ba, ok1 := placeEntity(tl, v, a, t).bbox(tl)
			bb, ok2 := placeEntity(tl, v, b, t).bbox(tl)
			if ok1 && ok2 && ba.intersects(bb) {
				return fmt.Errorf("expect %s, %s @%s: bounding boxes overlap", aName, bName, fmtTime(t))
			}
		}
	}
	if !foundA {
		return fmt.Errorf("expect: entity %q not found", aName)
	}
	if !foundB {
		return fmt.Errorf("expect: entity %q not found", bName)
	}
	return nil
}

// scopedViews narrows a scene-local expectation to its own scene. A
// global expectation searches every scene (first match wins for values,
// every occurrence for bounding-box checks).
func scopedViews(views []timelineView, exp expectation) []timelineView {
	if exp.scene >= 0 && exp.scene < len(views) {
		return views[exp.scene : exp.scene+1]
	}
	return views
}

func viewEntity(v timelineView, name string) (Entity, bool) {
	for _, e := range v.entities {
		if e.Name == name {
			return e, true
		}
	}
	return Entity{}, false
}

func entityOnCanvas(tl Timeline, v timelineView, e Entity, t float64) bool {
	w, h := tl.size()
	b, ok := placeEntity(tl, v, e, t).bbox(tl)
	return ok && b.overlapsCanvas(float64(w), float64(h))
}

// checkCanvas is the always-on sanity pass: a shape that is never
// visible is usually a wrong coordinate, while one that leaves the frame
// partway through is ordinary animation and only warns.
func checkCanvas(tl Timeline) ([]string, error) {
	views := timelineViews(tl)
	w, h := tl.size()

	// A global entity appears in every scene window; judge it over the
	// whole run so a shape visible in scene 2 isn't called dead because
	// scene 1 hasn't revealed it yet. Scene-owned shapes keep separate
	// records per scene, even when names repeat.
	type canvasStat struct {
		entity   Entity
		on, off  int
		valid    int
		firstOff float64
		haveOff  bool
	}
	stats := make(map[string]*canvasStat)
	var order []string
	for vi, v := range views {
		for ei, e := range v.entities {
			key := "scene" + strconv.Itoa(vi) + ":" + e.Name
			if ei < v.globals {
				key = "global:" + e.Name
			}
			st := stats[key]
			if st == nil {
				st = &canvasStat{entity: e}
				stats[key] = st
				order = append(order, key)
			}
			for _, t := range sampleTimes(v.start, v.end, expectStep) {
				b, ok := placeEntity(tl, v, e, t).bbox(tl)
				if !ok {
					// No geometry at all (e.g. an empty path) draws
					// nothing; there is no placement to judge.
					continue
				}
				st.valid++
				if b.overlapsCanvas(float64(w), float64(h)) {
					st.on++
					continue
				}
				st.off++
				if !st.haveOff {
					st.firstOff, st.haveOff = t, true
				}
			}
		}
	}

	var warnings []string
	for _, key := range order {
		st := stats[key]
		if st.valid == 0 {
			continue
		}
		if st.on == 0 {
			return nil, fmt.Errorf("entity %q (%s) is never on canvas (canvas %dx%d)",
				st.entity.Name, st.entity.Kind, w, h)
		}
		if st.off > 0 {
			warnings = append(warnings, fmt.Sprintf("entity %q (%s) is off canvas at %d/%d samples (first at %s)",
				st.entity.Name, st.entity.Kind, st.off, st.on+st.off, fmtTime(st.firstOff)))
		}
	}
	return warnings, nil
}

func valueEqual(got, want any) bool {
	if gf, ok := toFloat(got); ok {
		wf, ok := toFloat(want)
		return ok && math.Abs(gf-wf) <= expectTolerance
	}
	if gs, ok := got.(string); ok {
		ws, ok := want.(string)
		if !ok {
			return false
		}
		gc, gerr := parseColor(gs)
		wc, werr := parseColor(ws)
		if gerr == nil && werr == nil {
			return gc == wc
		}
		return gs == ws
	}
	if gb, ok := got.(bool); ok {
		wb, ok := want.(bool)
		return ok && gb == wb
	}
	if ga, ok := got.([]any); ok {
		wa, ok := want.([]any)
		if !ok || len(ga) != len(wa) {
			return false
		}
		for i := range ga {
			if !valueEqual(ga[i], wa[i]) {
				return false
			}
		}
		return true
	}
	return fmt.Sprint(got) == fmt.Sprint(want)
}

func fmtValue(v any, ok bool) string {
	if !ok {
		return "unset"
	}
	if f, ok := toFloat(v); ok {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return fmt.Sprint(v)
}

// fmtTime rounds away float drift from scene offsets so assertions read
// "@1.0s" like the docs promise, while still printing 1.25s and 0.05s
// exactly when a script uses them.
func fmtTime(t float64) string {
	s := strconv.FormatFloat(t, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-0" {
		s = "0"
	}
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s + "s"
}
