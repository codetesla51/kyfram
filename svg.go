package kyfram

// Static SVG import. This is a focused subset of the SVG document model:
// paths and basic shapes, groups, transforms, presentation styles, and
// solid/linear/radial paints. It deliberately ignores scripts, CSS
// selectors, filters, masks, text and external references. The goal is
// logos and icons, not a browser engine.

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fogleman/gg"
)

type svgDoc struct {
	Width      float64
	Height     float64
	ViewBox    [4]float64
	HasViewBox bool
	Preserve   string
	Shapes     []svgShape
	Gradients  map[string]*svgGradient
	IDs        map[string]*svgXMLNode
}

type svgShape struct {
	Contours      []pathContour
	Fill          string
	Stroke        string
	FillOpacity   float64
	StrokeOpacity float64
	Opacity       float64
	StrokeWidth   float64
	FillRule      string
	LineCap       string
	LineJoin      string
	Dash          []float64
	DashOffset    float64
	StrokeScale   float64
}

type svgStyle struct {
	fill          string
	stroke        string
	color         string
	fillOpacity   float64
	strokeOpacity float64
	strokeWidth   float64
	fillRule      string
	lineCap       string
	lineJoin      string
	dash          []float64
	dashOffset    float64
}

type svgGradient struct {
	ID    string
	Kind  string // linear or radial
	Units string // objectBoundingBox or userSpaceOnUse
	Href  string

	X1, Y1, X2, Y2 svgCoord
	CX, CY, R      svgCoord
	Stops          []svgStop
}

type svgStop struct {
	Offset  float64
	Color   string
	Opacity float64
}

type svgCoord struct {
	Value   float64
	Percent bool
}

func (c svgCoord) resolve(ref float64) float64 {
	if c.Percent {
		return c.Value / 100 * ref
	}
	return c.Value
}

type svgBox struct {
	X, Y, W, H float64
}

type svgMatrix struct {
	A, B, C, D, E, F float64
}

func svgIdentity() svgMatrix { return svgMatrix{A: 1, D: 1} }

func (m svgMatrix) mul(n svgMatrix) svgMatrix {
	return svgMatrix{
		A: m.A*n.A + m.C*n.B,
		B: m.B*n.A + m.D*n.B,
		C: m.A*n.C + m.C*n.D,
		D: m.B*n.C + m.D*n.D,
		E: m.A*n.E + m.C*n.F + m.E,
		F: m.B*n.E + m.D*n.F + m.F,
	}
}

func (m svgMatrix) apply(x, y float64) (float64, float64) {
	return m.A*x + m.C*y + m.E, m.B*x + m.D*y + m.F
}

func (m svgMatrix) det() float64 { return m.A*m.D - m.B*m.C }

type svgXMLNode struct {
	Name     string
	Attrs    map[string]string
	Children []*svgXMLNode
}

func (n *svgXMLNode) attr(key string) string {
	if v, ok := n.Attrs[key]; ok {
		return v
	}
	if v, ok := n.Attrs[strings.ToLower(key)]; ok {
		return v
	}
	return ""
}

func defaultSVGStyle() svgStyle {
	return svgStyle{
		fill: "#000000", stroke: "none", color: "#000000",
		fillOpacity: 1, strokeOpacity: 1, strokeWidth: 1,
		fillRule: "nonzero", lineCap: "butt", lineJoin: "miter",
	}
}

func (st svgStyle) merge(attrs map[string]string) svgStyle {
	get := func(k string) (string, bool) {
		if v, ok := attrs[k]; ok {
			return strings.TrimSpace(v), true
		}
		if v, ok := attrs[strings.ToLower(k)]; ok {
			return strings.TrimSpace(v), true
		}
		return "", false
	}
	out := st
	if v, ok := get("color"); ok && v != "" && v != "inherit" {
		out.color = v
	}
	setPaint := func(dst *string, key string) {
		if v, ok := get(key); ok && v != "" && v != "inherit" {
			if v == "currentColor" {
				v = out.color
			}
			*dst = v
		}
	}
	setPaint(&out.fill, "fill")
	setPaint(&out.stroke, "stroke")
	if v, ok := get("fill-opacity"); ok && v != "" && v != "inherit" {
		out.fillOpacity = parseSVGOpacityValue(v)
	}
	if v, ok := get("stroke-opacity"); ok && v != "" && v != "inherit" {
		out.strokeOpacity = parseSVGOpacityValue(v)
	}
	if v, ok := get("stroke-width"); ok && v != "" && v != "inherit" {
		out.strokeWidth = parseSVGNumber(v)
	}
	if v, ok := get("fill-rule"); ok && v != "" && v != "inherit" {
		out.fillRule = strings.ToLower(v)
	}
	if v, ok := get("stroke-linecap"); ok && v != "" && v != "inherit" {
		out.lineCap = strings.ToLower(v)
	}
	if v, ok := get("stroke-linejoin"); ok && v != "" && v != "inherit" {
		out.lineJoin = strings.ToLower(v)
	}
	if v, ok := get("stroke-dasharray"); ok && v != "" && v != "inherit" {
		out.dash = parseSVGDash(v)
	}
	if v, ok := get("stroke-dashoffset"); ok && v != "" && v != "inherit" {
		out.dashOffset = parseSVGNumber(v)
	}
	return out
}

func parseSVGOpacityValue(s string) float64 {
	if strings.TrimSpace(s) == "" {
		return 1
	}
	v := parseSVGNumber(s)
	if v > 1 {
		v = 1
	}
	if v < 0 {
		v = 0
	}
	return v
}

func parseSVGDash(s string) []float64 {
	if strings.EqualFold(strings.TrimSpace(s), "none") {
		return nil
	}
	nums, err := parseSVGNumberList(s)
	if err != nil || len(nums) == 0 {
		return nil
	}
	if len(nums)%2 == 1 {
		nums = append(nums, nums...)
	}
	for i := range nums {
		if nums[i] < 0 {
			nums[i] = 0
		}
	}
	return nums
}

func parseSVGNumber(s string) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "%") {
		v, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
		return v / 100
	}
	units := []struct {
		suffix string
		scale  float64
	}{
		{"px", 1}, {"pt", 96.0 / 72}, {"pc", 16},
		{"mm", 96.0 / 25.4}, {"cm", 96.0 / 2.54}, {"in", 96},
	}
	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			v, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, u.suffix)), 64)
			return v * u.scale
		}
	}
	v, _ := strconv.ParseFloat(s, 64)
	return v
}

func parseSVGLength(s string, ref float64) float64 {
	s = strings.TrimSpace(strings.ToLower(s))
	if strings.HasSuffix(s, "%") {
		v, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
		return v / 100 * ref
	}
	return parseSVGNumber(s)
}

func parseSVGCoord(s string) svgCoord {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "%") {
		v, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(s, "%")), 64)
		return svgCoord{Value: v, Percent: true}
	}
	return svgCoord{Value: parseSVGNumber(s)}
}

func parseSVGNumberList(s string) ([]float64, error) {
	p := &pathDParser{s: s}
	var out []float64
	for {
		p.skip()
		if p.eof() {
			return out, nil
		}
		v, err := p.number()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

func parseSVGXML(data []byte) (*svgXMLNode, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	root := &svgXMLNode{Name: "#root"}
	stack := []*svgXMLNode{root}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &svgXMLNode{Name: t.Name.Local, Attrs: map[string]string{}}
			for _, a := range t.Attr {
				n.Attrs[a.Name.Local] = a.Value
			}
			if style := n.Attrs["style"]; style != "" {
				for k, v := range parseSVGStyle(style) {
					n.Attrs[k] = v
				}
				delete(n.Attrs, "style")
			}
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	for _, c := range root.Children {
		if strings.EqualFold(c.Name, "svg") {
			return c, nil
		}
	}
	return nil, fmt.Errorf("no <svg> root")
}

func parseSVGStyle(s string) map[string]string {
	out := map[string]string{}
	for _, decl := range strings.Split(s, ";") {
		parts := strings.SplitN(decl, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		v = strings.TrimSpace(strings.TrimSuffix(v, "!important"))
		if k != "" && v != "" {
			out[k] = v
		}
	}
	return out
}

func parseSVG(data []byte) (*svgDoc, error) {
	root, err := parseSVGXML(data)
	if err != nil {
		return nil, err
	}
	doc := &svgDoc{Preserve: "xMidYMid meet", Gradients: map[string]*svgGradient{}, IDs: map[string]*svgXMLNode{}}
	if vb := root.attr("viewBox"); vb != "" {
		nums, err := parseSVGNumberList(vb)
		if err != nil || len(nums) != 4 || nums[2] <= 0 || nums[3] <= 0 {
			return nil, fmt.Errorf("invalid viewBox %q", vb)
		}
		doc.ViewBox = [4]float64{nums[0], nums[1], nums[2], nums[3]}
		doc.HasViewBox = true
	}
	vbW, vbH := doc.ViewBox[2], doc.ViewBox[3]
	if !doc.HasViewBox {
		vbW, vbH = 300, 150
	}
	if v := root.attr("width"); v != "" {
		doc.Width = parseSVGLength(v, vbW)
	}
	if v := root.attr("height"); v != "" {
		doc.Height = parseSVGLength(v, vbH)
	}
	if doc.Width <= 0 {
		if doc.HasViewBox {
			doc.Width = vbW
		} else {
			doc.Width = 300
		}
	}
	if doc.Height <= 0 {
		if doc.HasViewBox {
			doc.Height = vbH
		} else {
			doc.Height = 150
		}
	}
	if !doc.HasViewBox {
		doc.ViewBox = [4]float64{0, 0, doc.Width, doc.Height}
	}
	if v := root.attr("preserveAspectRatio"); v != "" {
		doc.Preserve = v
	}
	collectSVGGradients(root, doc.Gradients)
	inheritSVGGradients(doc.Gradients)
	collectSVGIDs(root, doc.IDs)

	b := svgBuilder{doc: doc}
	style := defaultSVGStyle().merge(root.Attrs)
	rootOpacity := parseSVGOpacityValue(root.attr("opacity"))
	for _, child := range root.Children {
		if err := b.walk(child, svgIdentity(), style, rootOpacity, 0); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

type svgBuilder struct {
	doc *svgDoc
}

func isSVGShape(name string) bool {
	switch name {
	case "path", "rect", "circle", "ellipse", "line", "polyline", "polygon":
		return true
	}
	return false
}

func svgHidden(n *svgXMLNode) bool {
	if strings.EqualFold(n.attr("display"), "none") {
		return true
	}
	switch strings.ToLower(n.attr("visibility")) {
	case "hidden", "collapse":
		return true
	}
	return false
}

func (b *svgBuilder) walk(n *svgXMLNode, parent svgMatrix, st svgStyle, opacity float64, depth int) error {
	if depth > 32 {
		return nil
	}
	name := strings.ToLower(n.Name)
	switch name {
	case "defs", "title", "desc", "metadata", "style", "script",
		"lineargradient", "radialgradient", "stop", "clippath", "mask",
		"pattern", "filter":
		return nil
	}
	if name == "use" {
		return b.walkUse(n, parent, st, opacity, depth)
	}
	if svgHidden(n) {
		return nil
	}
	if name == "g" || name == "a" || name == "svg" || name == "symbol" {
		st = st.merge(n.Attrs)
		m := parent
		if tr := n.attr("transform"); tr != "" {
			t, err := parseSVGTransform(tr)
			if err != nil {
				return fmt.Errorf("<%s> transform: %w", name, err)
			}
			m = parent.mul(t)
		}
		op := opacity * parseSVGOpacityValue(n.attr("opacity"))
		for _, child := range n.Children {
			if err := b.walk(child, m, st, op, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	if !isSVGShape(name) {
		return nil
	}
	st = st.merge(n.Attrs)
	m := parent
	if tr := n.attr("transform"); tr != "" {
		t, err := parseSVGTransform(tr)
		if err != nil {
			return fmt.Errorf("<%s> transform: %w", name, err)
		}
		m = parent.mul(t)
	}
	contours, err := b.shapeContours(name, n)
	if err != nil {
		return fmt.Errorf("<%s>: %w", name, err)
	}
	if len(contours) == 0 {
		return nil
	}
	for i := range contours {
		transformSVGContour(&contours[i], m)
	}
	scale := math.Sqrt(math.Abs(m.det()))
	if scale == 0 {
		scale = 1
	}
	fill, stroke := st.fill, st.stroke
	if name == "line" {
		fill = "none"
	}
	b.doc.Shapes = append(b.doc.Shapes, svgShape{
		Contours:      contours,
		Fill:          fill,
		Stroke:        stroke,
		FillOpacity:   st.fillOpacity,
		StrokeOpacity: st.strokeOpacity,
		Opacity:       opacity * parseSVGOpacityValue(n.attr("opacity")),
		StrokeWidth:   st.strokeWidth,
		FillRule:      st.fillRule,
		LineCap:       st.lineCap,
		LineJoin:      st.lineJoin,
		Dash:          st.dash,
		DashOffset:    st.dashOffset,
		StrokeScale:   scale,
	})
	return nil
}

func (b *svgBuilder) walkUse(n *svgXMLNode, parent svgMatrix, st svgStyle, opacity float64, depth int) error {
	href := strings.TrimSpace(n.attr("href"))
	if href == "" {
		return nil
	}
	id := strings.TrimPrefix(href, "#")
	target := b.doc.IDs[id]
	if target == nil {
		return nil
	}
	st = st.merge(n.Attrs)
	m := parent
	if tr := n.attr("transform"); tr != "" {
		t, err := parseSVGTransform(tr)
		if err != nil {
			return fmt.Errorf("<use> transform: %w", err)
		}
		m = parent.mul(t)
	}
	vbW, vbH := b.doc.ViewBox[2], b.doc.ViewBox[3]
	if vbW <= 0 {
		vbW = b.doc.Width
	}
	if vbH <= 0 {
		vbH = b.doc.Height
	}
	x := parseSVGLength(n.attr("x"), vbW)
	y := parseSVGLength(n.attr("y"), vbH)
	if x != 0 || y != 0 {
		m = m.mul(svgMatrix{A: 1, D: 1, E: x, F: y})
	}
	op := opacity * parseSVGOpacityValue(n.attr("opacity"))
	return b.walk(target, m, st, op, depth+1)
}

func (b *svgBuilder) shapeContours(name string, n *svgXMLNode) ([]pathContour, error) {
	vbW, vbH := b.doc.ViewBox[2], b.doc.ViewBox[3]
	if vbW <= 0 || vbH <= 0 {
		vbW, vbH = b.doc.Width, b.doc.Height
	}
	switch name {
	case "path":
		d := n.attr("d")
		if strings.TrimSpace(d) == "" {
			return nil, nil
		}
		contours, err := flattenPathD(d)
		if err != nil {
			return nil, err
		}
		// flattenPathD caches read-only contours; the builder transforms
		// points in place, so each SVG path needs its own copy.
		return clonePathContours(contours), nil

	case "rect":
		x := parseSVGLength(n.attr("x"), vbW)
		y := parseSVGLength(n.attr("y"), vbH)
		w := parseSVGLength(n.attr("width"), vbW)
		h := parseSVGLength(n.attr("height"), vbH)
		if w <= 0 || h <= 0 {
			return nil, nil
		}
		rx := parseSVGLength(n.attr("rx"), vbW)
		ry := parseSVGLength(n.attr("ry"), vbH)
		if rx > 0 && ry <= 0 {
			ry = rx
		}
		if ry > 0 && rx <= 0 {
			rx = ry
		}
		return svgRectContours(x, y, w, h, rx, ry), nil

	case "circle":
		cx := parseSVGLength(n.attr("cx"), vbW)
		cy := parseSVGLength(n.attr("cy"), vbH)
		r := parseSVGLength(n.attr("r"), math.Min(vbW, vbH))
		if r <= 0 {
			return nil, nil
		}
		return svgEllipseContours(cx, cy, r, r, 64), nil

	case "ellipse":
		cx := parseSVGLength(n.attr("cx"), vbW)
		cy := parseSVGLength(n.attr("cy"), vbH)
		rx := parseSVGLength(n.attr("rx"), vbW)
		ry := parseSVGLength(n.attr("ry"), vbH)
		if rx <= 0 || ry <= 0 {
			return nil, nil
		}
		return svgEllipseContours(cx, cy, rx, ry, 64), nil

	case "line":
		x1 := parseSVGLength(n.attr("x1"), vbW)
		y1 := parseSVGLength(n.attr("y1"), vbH)
		x2 := parseSVGLength(n.attr("x2"), vbW)
		y2 := parseSVGLength(n.attr("y2"), vbH)
		return []pathContour{{points: []float64{x1, y1, x2, y2}}}, nil

	case "polyline", "polygon":
		pts, err := parseSVGNumberList(n.attr("points"))
		if err != nil {
			return nil, err
		}
		if len(pts) < 4 {
			return nil, nil
		}
		if len(pts)%2 != 0 {
			pts = pts[:len(pts)-1]
		}
		return []pathContour{{points: pts, closed: name == "polygon"}}, nil
	}
	return nil, nil
}

func clonePathContours(contours []pathContour) []pathContour {
	if len(contours) == 0 {
		return nil
	}
	out := make([]pathContour, len(contours))
	for i, c := range contours {
		out[i] = pathContour{points: append([]float64(nil), c.points...), closed: c.closed}
	}
	return out
}

func transformSVGContour(c *pathContour, m svgMatrix) {
	for i := 0; i+1 < len(c.points); i += 2 {
		c.points[i], c.points[i+1] = m.apply(c.points[i], c.points[i+1])
	}
}

func svgRectContours(x, y, w, h, rx, ry float64) []pathContour {
	if rx <= 0 || ry <= 0 {
		return []pathContour{{
			points: []float64{x, y, x + w, y, x + w, y + h, x, y + h},
			closed: true,
		}}
	}
	if rx > w/2 {
		rx = w / 2
	}
	if ry > h/2 {
		ry = h / 2
	}
	const k = 0.5522847498307936
	pts := []float64{x + rx, y}
	addLine := func(px, py float64) {
		pts = append(pts, px, py)
	}
	addCubic := func(p0, p1, p2, p3 [2]float64) {
		flattenCubic(&pts, p0, p1, p2, p3, 0.35, 0)
	}
	addLine(x+w-rx, y)
	addCubic(
		[2]float64{x + w - rx, y},
		[2]float64{x + w - rx + k*rx, y},
		[2]float64{x + w, y + ry - k*ry},
		[2]float64{x + w, y + ry},
	)
	addLine(x+w, y+h-ry)
	addCubic(
		[2]float64{x + w, y + h - ry},
		[2]float64{x + w, y + h - ry + k*ry},
		[2]float64{x + w - rx + k*rx, y + h},
		[2]float64{x + w - rx, y + h},
	)
	addLine(x+rx, y+h)
	addCubic(
		[2]float64{x + rx, y + h},
		[2]float64{x + rx - k*rx, y + h},
		[2]float64{x, y + h - ry + k*ry},
		[2]float64{x, y + h - ry},
	)
	addLine(x, y+ry)
	addCubic(
		[2]float64{x, y + ry},
		[2]float64{x, y + ry - k*ry},
		[2]float64{x + rx - k*rx, y},
		[2]float64{x + rx, y},
	)
	return []pathContour{{points: pts, closed: true}}
}

func svgEllipseContours(cx, cy, rx, ry float64, segments int) []pathContour {
	if segments < 12 {
		segments = 12
	}
	pts := make([]float64, 0, segments*2)
	for i := 0; i < segments; i++ {
		t := 2 * math.Pi * float64(i) / float64(segments)
		pts = append(pts, cx+rx*math.Cos(t), cy+ry*math.Sin(t))
	}
	return []pathContour{{points: pts, closed: true}}
}

func parseSVGTransform(s string) (svgMatrix, error) {
	m := svgIdentity()
	for i := 0; i < len(s); {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r' || s[i] == ',') {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		for i < len(s) && ((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) {
			i++
		}
		if start == i {
			return m, fmt.Errorf("expected transform name at %q", s[i:])
		}
		name := strings.ToLower(s[start:i])
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) || s[i] != '(' {
			return m, fmt.Errorf("expected '(' after %s", name)
		}
		j := strings.IndexByte(s[i:], ')')
		if j < 0 {
			return m, fmt.Errorf("unclosed %s()", name)
		}
		args, err := parseSVGNumberList(s[i+1 : i+j])
		if err != nil {
			return m, err
		}
		i += j + 1

		var t svgMatrix
		switch name {
		case "translate":
			if len(args) < 1 {
				return m, fmt.Errorf("translate needs at least one value")
			}
			tx := args[0]
			ty := 0.0
			if len(args) >= 2 {
				ty = args[1]
			}
			t = svgMatrix{A: 1, D: 1, E: tx, F: ty}
		case "scale":
			if len(args) < 1 {
				return m, fmt.Errorf("scale needs at least one value")
			}
			sx := args[0]
			sy := sx
			if len(args) >= 2 {
				sy = args[1]
			}
			t = svgMatrix{A: sx, D: sy}
		case "rotate":
			if len(args) != 1 && len(args) != 3 {
				return m, fmt.Errorf("rotate needs 1 or 3 values")
			}
			a := args[0] * math.Pi / 180
			c, sn := math.Cos(a), math.Sin(a)
			t = svgMatrix{A: c, B: sn, C: -sn, D: c}
			if len(args) == 3 {
				cx, cy := args[1], args[2]
				t = svgMatrix{A: 1, D: 1, E: cx, F: cy}.mul(t).mul(svgMatrix{A: 1, D: 1, E: -cx, F: -cy})
			}
		case "skewx":
			if len(args) < 1 {
				return m, fmt.Errorf("skewX needs one value")
			}
			t = svgMatrix{A: 1, C: math.Tan(args[0] * math.Pi / 180), D: 1}
		case "skewy":
			if len(args) < 1 {
				return m, fmt.Errorf("skewY needs one value")
			}
			t = svgMatrix{A: 1, B: math.Tan(args[0] * math.Pi / 180), D: 1}
		case "matrix":
			if len(args) != 6 {
				return m, fmt.Errorf("matrix needs six values")
			}
			t = svgMatrix{A: args[0], B: args[1], C: args[2], D: args[3], E: args[4], F: args[5]}
		default:
			return m, fmt.Errorf("unsupported transform %q", name)
		}
		m = m.mul(t)
	}
	return m, nil
}

func collectSVGIDs(n *svgXMLNode, out map[string]*svgXMLNode) {
	if id := n.attr("id"); id != "" {
		out[id] = n
	}
	for _, c := range n.Children {
		collectSVGIDs(c, out)
	}
}

func collectSVGGradients(n *svgXMLNode, out map[string]*svgGradient) {
	name := strings.ToLower(n.Name)
	if name == "lineargradient" || name == "radialgradient" {
		if g := parseSVGGradient(n); g != nil && g.ID != "" {
			out[g.ID] = g
		}
	}
	for _, c := range n.Children {
		collectSVGGradients(c, out)
	}
}

func parseSVGGradient(n *svgXMLNode) *svgGradient {
	g := &svgGradient{ID: n.attr("id"), Units: n.attr("gradientUnits"), Href: n.attr("href")}
	if g.ID == "" {
		return nil
	}
	if g.Units == "" {
		g.Units = "objectBoundingBox"
	}
	switch strings.ToLower(n.Name) {
	case "lineargradient":
		g.Kind = "linear"
		g.X1 = parseSVGCoordOr(n.attr("x1"), "0%")
		g.Y1 = parseSVGCoordOr(n.attr("y1"), "0%")
		g.X2 = parseSVGCoordOr(n.attr("x2"), "100%")
		g.Y2 = parseSVGCoordOr(n.attr("y2"), "0%")
	case "radialgradient":
		g.Kind = "radial"
		g.CX = parseSVGCoordOr(n.attr("cx"), "50%")
		g.CY = parseSVGCoordOr(n.attr("cy"), "50%")
		g.R = parseSVGCoordOr(n.attr("r"), "50%")
	default:
		return nil
	}
	for _, c := range n.Children {
		if !strings.EqualFold(c.Name, "stop") {
			continue
		}
		off := parseSVGCoord(c.attr("offset"))
		v := off.Value
		if off.Percent {
			v /= 100
		}
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		col := c.attr("stop-color")
		if col == "" {
			col = "#000000"
		}
		op := 1.0
		if c.attr("stop-opacity") != "" {
			op = parseSVGOpacityValue(c.attr("stop-opacity"))
		}
		g.Stops = append(g.Stops, svgStop{Offset: v, Color: col, Opacity: op})
	}
	return g
}

func parseSVGCoordOr(s, def string) svgCoord {
	if strings.TrimSpace(s) == "" {
		s = def
	}
	return parseSVGCoord(s)
}

func inheritSVGGradients(grads map[string]*svgGradient) {
	for _, g := range grads {
		if len(g.Stops) != 0 || g.Href == "" {
			continue
		}
		base := grads[strings.TrimPrefix(g.Href, "#")]
		if base != nil {
			g.Stops = base.Stops
		}
	}
}

func loadSVG(path string) (*svgDoc, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if v, ok := svgCache.Load(path); ok {
		e := v.(svgCacheEntry)
		if e.mod.Equal(fi.ModTime()) && e.size == fi.Size() {
			return e.doc, e.err
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	doc, err := parseSVG(data)
	svgCache.Store(path, svgCacheEntry{doc: doc, err: err, mod: fi.ModTime(), size: fi.Size()})
	return doc, err
}

type svgCacheEntry struct {
	doc  *svgDoc
	err  error
	mod  time.Time
	size int64
}

var svgCache sync.Map

func drawSVGDoc(dc *gg.Context, doc *svgDoc, x, y, w, h, alpha float64) {
	if doc == nil || w <= 0 || h <= 0 || alpha <= 0 {
		return
	}
	if alpha > 1 {
		alpha = 1
	}
	// Rasterize the document once in its own coordinate space, then place
	// the image under the caller's transform. gg's gradient patterns are
	// device-space, so drawing vector gradients directly would not follow
	// entity/group/camera transforms.
	// Supersample normal-sized entities so camera zoom and animated scale
	// stay crisp. Very large targets fall back to 1x to cap memory use.
	ss := 2.0
	if w > 2048 || h > 2048 {
		ss = 1
	}
	iw := int(math.Ceil(w * ss))
	ih := int(math.Ceil(h * ss))
	if iw < 1 || ih < 1 {
		return
	}
	layer := gg.NewContext(iw, ih)
	drawSVGVector(layer, doc, float64(iw), float64(ih), 1)
	im := layer.Image()
	if alpha < 1 {
		// Apply entity alpha once to the finished layer, not to each shape;
		// otherwise overlapping parts of the SVG would darken differently.
		dst := image.NewNRGBA(im.Bounds())
		draw.Draw(dst, dst.Bounds(), im, im.Bounds().Min, draw.Src)
		for i := 3; i < len(dst.Pix); i += 4 {
			dst.Pix[i] = uint8(float64(dst.Pix[i])*alpha + 0.5)
		}
		im = dst
	}
	dc.Push()
	dc.Translate(x, y)
	if ss != 1 {
		dc.Scale(1/ss, 1/ss)
	}
	dc.DrawImageAnchored(im, 0, 0, 0, 0)
	dc.Pop()
}

func drawSVGVector(dc *gg.Context, doc *svgDoc, w, h, alpha float64) {
	vb := doc.ViewBox
	if vb[2] <= 0 || vb[3] <= 0 {
		return
	}
	sx, sy := w/vb[2], h/vb[3]
	align := "xMidYMid"
	mode := "meet"
	parts := strings.Fields(doc.Preserve)
	if len(parts) > 0 {
		align = parts[0]
	}
	if len(parts) > 1 {
		mode = parts[1]
	}
	ox, oy := 0.0, 0.0
	if !strings.EqualFold(align, "none") {
		s := math.Min(sx, sy)
		if strings.EqualFold(mode, "slice") {
			s = math.Max(sx, sy)
		}
		sx, sy = s, s
		extraX := w - vb[2]*s
		extraY := h - vb[3]*s
		ox = svgAlignOffset(align, extraX, 0)
		oy = svgAlignOffset(align, extraY, 1)
	}
	root := svgMatrix{A: sx, D: sy, E: ox - vb[0]*sx, F: oy - vb[1]*sy}
	dc.Push()
	dc.Translate(ox, oy)
	dc.Scale(sx, sy)
	dc.Translate(-vb[0], -vb[1])
	for _, sh := range doc.Shapes {
		drawSVGShape(dc, doc, sh, alpha, root)
	}
	dc.Pop()
}

func svgAlignOffset(align string, extra float64, axis int) float64 {
	if len(align) < 8 {
		return extra / 2
	}
	var part string
	if axis == 0 {
		part = align[:4]
	} else {
		part = align[4:8]
	}
	switch part {
	case "xMin", "YMin":
		return 0
	case "xMax", "YMax":
		return extra
	}
	return extra / 2
}

func drawSVGShape(dc *gg.Context, doc *svgDoc, sh svgShape, alpha float64, root svgMatrix) {
	if len(sh.Contours) == 0 {
		return
	}
	hasFill := sh.Fill != "" && !strings.EqualFold(sh.Fill, "none")
	hasStroke := sh.Stroke != "" && !strings.EqualFold(sh.Stroke, "none") && sh.StrokeWidth > 0
	if !hasFill && !hasStroke {
		return
	}
	box, _ := svgContoursBox(sh.Contours)
	opacity := sh.Opacity * alpha
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}

	dc.Push()
	defer dc.Pop()
	if hasFill {
		traceContours(dc, sh.Contours, false)
		if strings.EqualFold(sh.FillRule, "evenodd") {
			dc.SetFillRule(gg.FillRuleEvenOdd)
		} else {
			dc.SetFillRule(gg.FillRuleWinding)
		}
		if svgSetPaint(dc, doc, sh.Fill, sh.FillOpacity*opacity, box, true, root) {
			dc.FillPreserve()
		} else {
			dc.ClearPath()
			hasFill = false
		}
	}
	if hasStroke {
		if !hasFill {
			traceContours(dc, sh.Contours, false)
		}
		dc.SetLineWidth(sh.StrokeWidth * sh.StrokeScale)
		switch strings.ToLower(sh.LineCap) {
		case "round":
			dc.SetLineCapRound()
		case "square":
			dc.SetLineCapSquare()
		default:
			dc.SetLineCapButt()
		}
		switch strings.ToLower(sh.LineJoin) {
		case "round":
			dc.SetLineJoinRound()
		case "bevel":
			dc.SetLineJoinBevel()
		default:
			// gg has no miter joiner; bevel is the closest fallback.
			dc.SetLineJoin(gg.LineJoinBevel)
		}
		if len(sh.Dash) > 0 {
			dashes := make([]float64, len(sh.Dash))
			for i, d := range sh.Dash {
				dashes[i] = d * sh.StrokeScale
			}
			dc.SetDash(dashes...)
			dc.SetDashOffset(sh.DashOffset * sh.StrokeScale)
		} else {
			dc.SetDash()
			dc.SetDashOffset(0)
		}
		if svgSetPaint(dc, doc, sh.Stroke, sh.StrokeOpacity*opacity, box, false, root) {
			dc.Stroke()
		} else {
			dc.ClearPath()
		}
	} else if hasFill {
		dc.ClearPath()
	}
}

func svgContoursBox(contours []pathContour) (svgBox, bool) {
	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for _, c := range contours {
		for i := 0; i+1 < len(c.points); i += 2 {
			x, y := c.points[i], c.points[i+1]
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}
		}
	}
	if math.IsInf(minX, 1) {
		return svgBox{}, false
	}
	return svgBox{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}, true
}

func svgSetPaint(dc *gg.Context, doc *svgDoc, spec string, opacity float64, box svgBox, fill bool, root svgMatrix) bool {
	spec = strings.TrimSpace(spec)
	if spec == "" || strings.EqualFold(spec, "none") {
		return false
	}
	if strings.HasPrefix(strings.ToLower(spec), "url(") {
		end := strings.IndexByte(spec, ')')
		if end < 0 {
			return false
		}
		id := strings.TrimSpace(spec[4:end])
		id = strings.TrimPrefix(id, "#")
		g := svgGradientPattern(doc, id, box, opacity, root)
		if g == nil {
			return false
		}
		if fill {
			dc.SetFillStyle(g)
		} else {
			dc.SetStrokeStyle(g)
		}
		return true
	}
	c, ok := parseSVGColor(spec)
	if !ok {
		return false
	}
	a := opacity * c[3] / 255
	if a < 0 {
		a = 0
	}
	if a > 1 {
		a = 1
	}
	if fill {
		dc.SetRGBA(c[0]/255, c[1]/255, c[2]/255, a)
	} else {
		dc.SetRGBA(c[0]/255, c[1]/255, c[2]/255, a)
	}
	return true
}

func svgGradientPattern(doc *svgDoc, id string, box svgBox, opacity float64, root svgMatrix) gg.Pattern {
	if doc == nil {
		return nil
	}
	g := doc.Gradients[id]
	if g == nil || len(g.Stops) == 0 {
		return nil
	}
	w, h := box.W, box.H
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	user := strings.EqualFold(g.Units, "userSpaceOnUse")
	vbW, vbH := doc.ViewBox[2], doc.ViewBox[3]
	if vbW <= 0 {
		vbW = doc.Width
	}
	if vbH <= 0 {
		vbH = doc.Height
	}
	p := &svgPaintPattern{}
	switch g.Kind {
	case "linear":
		var x1, y1, x2, y2 float64
		if user {
			x1 = g.X1.resolve(vbW)
			y1 = g.Y1.resolve(vbH)
			x2 = g.X2.resolve(vbW)
			y2 = g.Y2.resolve(vbH)
		} else {
			x1 = box.X + g.X1.resolve(1)*w
			y1 = box.Y + g.Y1.resolve(1)*h
			x2 = box.X + g.X2.resolve(1)*w
			y2 = box.Y + g.Y2.resolve(1)*h
		}
		x1, y1 = root.apply(x1, y1)
		x2, y2 = root.apply(x2, y2)
		p.Linear = true
		p.X0, p.Y0, p.X1, p.Y1 = x1, y1, x2, y2
	case "radial":
		var cx, cy, r float64
		if user {
			cx = g.CX.resolve(vbW)
			cy = g.CY.resolve(vbH)
			r = g.R.resolve(math.Hypot(vbW, vbH) / math.Sqrt2)
		} else {
			cx = box.X + g.CX.resolve(1)*w
			cy = box.Y + g.CY.resolve(1)*h
			r = g.R.resolve(1) * math.Hypot(w, h) / math.Sqrt2
		}
		if r <= 0 {
			return nil
		}
		cx, cy = root.apply(cx, cy)
		r *= math.Sqrt(math.Abs(root.det()))
		p.CX, p.CY, p.R = cx, cy, r
	default:
		return nil
	}
	stops := append([]svgStop(nil), g.Stops...)
	sort.SliceStable(stops, func(i, j int) bool { return stops[i].Offset < stops[j].Offset })
	for _, s := range stops {
		c, ok := parseSVGColor(s.Color)
		if strings.EqualFold(strings.TrimSpace(s.Color), "transparent") {
			c, ok = [4]float64{0, 0, 0, 0}, true
		}
		if !ok {
			continue
		}
		a := opacity * s.Opacity * c[3] / 255
		if a < 0 {
			a = 0
		}
		if a > 1 {
			a = 1
		}
		p.Stops = append(p.Stops, svgPaintStop{
			Offset: s.Offset,
			R:      c[0], G: c[1], B: c[2], A: a * 255,
		})
	}
	if len(p.Stops) == 0 {
		return nil
	}
	return p
}

// svgPaintPattern is a small gradient implementation on top of gg.Pattern.
// gg's built-in gradients interpolate premultiplied RGBA incorrectly for
// translucent stops; doing the interpolation here keeps SVG opacity clean.
type svgPaintPattern struct {
	Linear bool

	X0, Y0, X1, Y1 float64
	CX, CY, R      float64
	Stops          []svgPaintStop
}

type svgPaintStop struct {
	Offset  float64
	R, G, B float64
	A       float64
}

func (p *svgPaintPattern) ColorAt(x, y int) color.Color {
	t := 0.0
	if p.Linear {
		dx, dy := p.X1-p.X0, p.Y1-p.Y0
		den := dx*dx + dy*dy
		if den != 0 {
			t = ((float64(x)-p.X0)*dx + (float64(y)-p.Y0)*dy) / den
		}
	} else {
		if p.R <= 0 {
			return color.NRGBA{}
		}
		t = math.Hypot(float64(x)-p.CX, float64(y)-p.CY) / p.R
	}
	return p.colorAt(t)
}

func (p *svgPaintPattern) colorAt(t float64) color.Color {
	if len(p.Stops) == 0 {
		return color.NRGBA{}
	}
	if t <= p.Stops[0].Offset {
		return svgStopColor(p.Stops[0])
	}
	last := p.Stops[len(p.Stops)-1]
	if t >= last.Offset {
		return svgStopColor(last)
	}
	for i := 1; i < len(p.Stops); i++ {
		b := p.Stops[i]
		if t >= b.Offset {
			continue
		}
		a := p.Stops[i-1]
		den := b.Offset - a.Offset
		f := 0.0
		if den != 0 {
			f = (t - a.Offset) / den
		}
		return color.NRGBA{
			R: uint8(a.R + (b.R-a.R)*f + 0.5),
			G: uint8(a.G + (b.G-a.G)*f + 0.5),
			B: uint8(a.B + (b.B-a.B)*f + 0.5),
			A: uint8(a.A + (b.A-a.A)*f + 0.5),
		}
	}
	return svgStopColor(last)
}

func svgStopColor(s svgPaintStop) color.NRGBA {
	return color.NRGBA{
		R: uint8(s.R + 0.5),
		G: uint8(s.G + 0.5),
		B: uint8(s.B + 0.5),
		A: uint8(s.A + 0.5),
	}
}

func parseSVGColor(s string) ([4]float64, bool) {
	s = strings.TrimSpace(strings.ToLower(s))
	var out [4]float64
	if s == "" || s == "none" || s == "transparent" {
		return out, false
	}
	if s == "currentcolor" {
		s = "#000000"
	}
	if strings.HasPrefix(s, "#") {
		h := s[1:]
		switch len(h) {
		case 3, 4:
			expanded := make([]byte, 0, len(h)*2)
			for i := 0; i < len(h); i++ {
				expanded = append(expanded, h[i], h[i])
			}
			h = string(expanded)
		}
		if len(h) != 6 && len(h) != 8 {
			return out, false
		}
		for i := 0; i < len(h); i += 2 {
			n, err := strconv.ParseUint(h[i:i+2], 16, 8)
			if err != nil {
				return out, false
			}
			out[i/2] = float64(n)
		}
		if len(h) == 6 {
			out[3] = 255
		}
		return out, true
	}
	if strings.HasPrefix(s, "rgb(") || strings.HasPrefix(s, "rgba(") {
		open := strings.IndexByte(s, '(')
		close := strings.LastIndexByte(s, ')')
		if open < 0 || close <= open {
			return out, false
		}
		body := s[open+1 : close]
		body = strings.ReplaceAll(body, "/", " ")
		body = strings.ReplaceAll(body, ",", " ")
		fields := strings.Fields(body)
		if len(fields) < 3 || len(fields) > 4 {
			return out, false
		}
		for i := 0; i < 3; i++ {
			v, ok := parseSVGColorChannel(fields[i])
			if !ok {
				return out, false
			}
			out[i] = v
		}
		out[3] = 255
		if len(fields) == 4 {
			a := parseSVGNumber(fields[3])
			if !strings.HasSuffix(fields[3], "%") && a > 1 {
				a /= 255
			}
			if a < 0 {
				a = 0
			}
			if a > 1 {
				a = 1
			}
			out[3] = a * 255
		}
		return out, true
	}
	c, err := parseColor(s)
	if err != nil {
		return out, false
	}
	out[0], out[1], out[2], out[3] = c[0], c[1], c[2], 255
	return out, true
}

func parseSVGColorChannel(s string) (float64, bool) {
	if strings.HasSuffix(s, "%") {
		v, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
		if err != nil {
			return 0, false
		}
		return math.Max(0, math.Min(255, v*2.55)), true
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return math.Max(0, math.Min(255, v)), true
}
