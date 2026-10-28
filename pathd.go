package kyfram

import (
	"fmt"
	"math"
	"strconv"
	"sync"
)

// pathContour is one flattened subpath. Points are [x0,y0,x1,y1,...].
type pathContour struct {
	points []float64
	closed bool
}

type pathDEntry struct {
	contours []pathContour
	err      error
}

// Parsed d strings are cached because keyframes hold one string for many
// frames. The slices are read-only after construction.
var pathDCache sync.Map

// flattenPathD parses and flattens SVG path data: M/L/H/V/C/S/Q/T/A/Z,
// absolute or relative. Curves and arcs are flattened adaptively at render
// time so the script only declares control points.
func flattenPathD(d string) ([]pathContour, error) {
	if v, ok := pathDCache.Load(d); ok {
		e := v.(pathDEntry)
		return e.contours, e.err
	}
	contours, err := parsePathD(d)
	pathDCache.Store(d, pathDEntry{contours: contours, err: err})
	return contours, err
}

func parsePathD(d string) ([]pathContour, error) {
	p := &pathDParser{s: d}
	var contours []pathContour
	var cur *pathContour
	var cx, cy, sx, sy float64
	var cmd, prevCmd byte
	var haveCmd, moved bool
	var lastCubic, lastQuad [2]float64

	finish := func() {
		if cur != nil && len(cur.points) >= 4 {
			contours = append(contours, *cur)
		}
		cur = nil
	}
	start := func(x, y float64) {
		finish()
		cur = &pathContour{points: []float64{x, y}}
		cx, cy, sx, sy = x, y, x, y
		moved = true
	}
	ensure := func() error {
		if cur != nil {
			return nil
		}
		if !moved {
			return fmt.Errorf("d: command %q before M", cmd)
		}
		// After Z the current point is the subpath start; a following
		// draw command continues from there in a fresh contour.
		cur = &pathContour{points: []float64{cx, cy}}
		return nil
	}
	addPoint := func(x, y float64) {
		cur.points = append(cur.points, x, y)
		cx, cy = x, y
	}

	for {
		p.skip()
		if p.eof() {
			break
		}
		c := p.s[p.i]
		if isPathCommand(c) {
			cmd = c
			haveCmd = true
			p.i++
			if cmd == 'Z' || cmd == 'z' {
				if cur == nil {
					return nil, fmt.Errorf("d: Z before M")
				}
				cur.closed = true
				finish()
				cx, cy = sx, sy
				prevCmd = 'Z'
				continue
			}
		} else if !p.hasNumber() {
			return nil, fmt.Errorf("d: unexpected %q at byte %d", c, p.i)
		}
		if !haveCmd {
			return nil, fmt.Errorf("d: must start with M")
		}

		rel := cmd >= 'a' && cmd <= 'z'
		switch cmd {
		case 'M', 'm':
			x, err := p.number()
			if err != nil {
				return nil, err
			}
			y, err := p.number()
			if err != nil {
				return nil, err
			}
			if rel {
				x, y = cx+x, cy+y
			}
			start(x, y)
			prevCmd = 'M'
			// Extra pairs after M are implicit L.
			for p.nextIsNumber() {
				x, err = p.number()
				if err != nil {
					return nil, err
				}
				y, err = p.number()
				if err != nil {
					return nil, err
				}
				if rel {
					x, y = cx+x, cy+y
				}
				addPoint(x, y)
				prevCmd = 'L'
			}

		case 'L', 'l':
			if err := ensure(); err != nil {
				return nil, err
			}
			if !p.nextIsNumber() {
				return nil, fmt.Errorf("d: L needs coordinates")
			}
			for p.nextIsNumber() {
				x, err := p.number()
				if err != nil {
					return nil, err
				}
				y, err := p.number()
				if err != nil {
					return nil, err
				}
				if rel {
					x, y = cx+x, cy+y
				}
				addPoint(x, y)
				prevCmd = 'L'
			}

		case 'H', 'h':
			if err := ensure(); err != nil {
				return nil, err
			}
			if !p.nextIsNumber() {
				return nil, fmt.Errorf("d: H needs a coordinate")
			}
			for p.nextIsNumber() {
				x, err := p.number()
				if err != nil {
					return nil, err
				}
				if rel {
					x = cx + x
				}
				addPoint(x, cy)
				prevCmd = 'H'
			}

		case 'V', 'v':
			if err := ensure(); err != nil {
				return nil, err
			}
			if !p.nextIsNumber() {
				return nil, fmt.Errorf("d: V needs a coordinate")
			}
			for p.nextIsNumber() {
				y, err := p.number()
				if err != nil {
					return nil, err
				}
				if rel {
					y = cy + y
				}
				addPoint(cx, y)
				prevCmd = 'V'
			}

		case 'C', 'c':
			if err := ensure(); err != nil {
				return nil, err
			}
			if !p.nextIsNumber() {
				return nil, fmt.Errorf("d: C needs coordinates")
			}
			for p.nextIsNumber() {
				x1, err := p.number()
				if err != nil {
					return nil, err
				}
				y1, err := p.number()
				if err != nil {
					return nil, err
				}
				x2, err := p.number()
				if err != nil {
					return nil, err
				}
				y2, err := p.number()
				if err != nil {
					return nil, err
				}
				x, err := p.number()
				if err != nil {
					return nil, err
				}
				y, err := p.number()
				if err != nil {
					return nil, err
				}
				if rel {
					x1, y1 = cx+x1, cy+y1
					x2, y2 = cx+x2, cy+y2
					x, y = cx+x, cy+y
				}
				flattenCubic(&cur.points,
					[2]float64{cx, cy},
					[2]float64{x1, y1},
					[2]float64{x2, y2},
					[2]float64{x, y},
					0.35, 0)
				cx, cy = x, y
				lastCubic = [2]float64{x2, y2}
				prevCmd = 'C'
			}

		case 'S', 's':
			if err := ensure(); err != nil {
				return nil, err
			}
			if !p.nextIsNumber() {
				return nil, fmt.Errorf("d: S needs coordinates")
			}
			for p.nextIsNumber() {
				x2, err := p.number()
				if err != nil {
					return nil, err
				}
				y2, err := p.number()
				if err != nil {
					return nil, err
				}
				x, err := p.number()
				if err != nil {
					return nil, err
				}
				y, err := p.number()
				if err != nil {
					return nil, err
				}
				cp1 := [2]float64{cx, cy}
				if prevCmd == 'C' || prevCmd == 'c' || prevCmd == 'S' || prevCmd == 's' {
					cp1 = [2]float64{2*cx - lastCubic[0], 2*cy - lastCubic[1]}
				}
				if rel {
					x2, y2 = cx+x2, cy+y2
					x, y = cx+x, cy+y
				}
				flattenCubic(&cur.points,
					[2]float64{cx, cy}, cp1,
					[2]float64{x2, y2}, [2]float64{x, y},
					0.35, 0)
				cx, cy = x, y
				lastCubic = [2]float64{x2, y2}
				prevCmd = 'S'
			}

		case 'Q', 'q':
			if err := ensure(); err != nil {
				return nil, err
			}
			if !p.nextIsNumber() {
				return nil, fmt.Errorf("d: Q needs coordinates")
			}
			for p.nextIsNumber() {
				qx, err := p.number()
				if err != nil {
					return nil, err
				}
				qy, err := p.number()
				if err != nil {
					return nil, err
				}
				x, err := p.number()
				if err != nil {
					return nil, err
				}
				y, err := p.number()
				if err != nil {
					return nil, err
				}
				if rel {
					qx, qy = cx+qx, cy+qy
					x, y = cx+x, cy+y
				}
				quadToCubic(&cur.points, [2]float64{cx, cy}, [2]float64{qx, qy}, [2]float64{x, y})
				cx, cy = x, y
				lastQuad = [2]float64{qx, qy}
				prevCmd = 'Q'
			}

		case 'T', 't':
			if err := ensure(); err != nil {
				return nil, err
			}
			if !p.nextIsNumber() {
				return nil, fmt.Errorf("d: T needs coordinates")
			}
			for p.nextIsNumber() {
				x, err := p.number()
				if err != nil {
					return nil, err
				}
				y, err := p.number()
				if err != nil {
					return nil, err
				}
				q := [2]float64{cx, cy}
				if prevCmd == 'Q' || prevCmd == 'q' || prevCmd == 'T' || prevCmd == 't' {
					q = [2]float64{2*cx - lastQuad[0], 2*cy - lastQuad[1]}
				}
				if rel {
					x, y = cx+x, cy+y
				}
				quadToCubic(&cur.points, [2]float64{cx, cy}, q, [2]float64{x, y})
				cx, cy = x, y
				lastQuad = q
				prevCmd = 'T'
			}

		case 'A', 'a':
			if err := ensure(); err != nil {
				return nil, err
			}
			if !p.nextIsNumber() {
				return nil, fmt.Errorf("d: A needs parameters")
			}
			for {
				rx, err := p.number()
				if err != nil {
					return nil, err
				}
				ry, err := p.number()
				if err != nil {
					return nil, err
				}
				rot, err := p.number()
				if err != nil {
					return nil, err
				}
				large, err := p.flag()
				if err != nil {
					return nil, err
				}
				sweep, err := p.flag()
				if err != nil {
					return nil, err
				}
				x, err := p.number()
				if err != nil {
					return nil, err
				}
				y, err := p.number()
				if err != nil {
					return nil, err
				}
				if rel {
					x, y = cx+x, cy+y
				}
				if rx < 0 || ry < 0 {
					return nil, fmt.Errorf("d: arc radii must not be negative")
				}
				appendArc(&cur.points, [2]float64{cx, cy}, [2]float64{x, y}, rx, ry, rot, large, sweep)
				cx, cy = x, y
				prevCmd = 'A'
				if !p.nextIsNumber() {
					break
				}
			}

		default:
			return nil, fmt.Errorf("d: unsupported command %q", string(cmd))
		}
	}
	finish()
	if len(contours) == 0 {
		return nil, fmt.Errorf("d: no drawable path")
	}
	return contours, nil
}

// quadToCubic converts a quadratic Bézier to cubic and flattens it.
func quadToCubic(out *[]float64, p0, q, p1 [2]float64) {
	cp1 := [2]float64{p0[0] + 2.0/3.0*(q[0]-p0[0]), p0[1] + 2.0/3.0*(q[1]-p0[1])}
	cp2 := [2]float64{p1[0] + 2.0/3.0*(q[0]-p1[0]), p1[1] + 2.0/3.0*(q[1]-p1[1])}
	flattenCubic(out, p0, cp1, cp2, p1, 0.35, 0)
}

// appendArc converts an SVG endpoint-parameterized elliptical arc into
// cubic segments, then flattens each segment. The endpoint is written
// exactly so path bounds and later commands stay stable.
func appendArc(out *[]float64, p0, p1 [2]float64, rx, ry, rotation float64, large, sweep bool) {
	if rx == 0 || ry == 0 {
		*out = append(*out, p1[0], p1[1])
		return
	}
	if p0 == p1 {
		return
	}
	rx, ry = math.Abs(rx), math.Abs(ry)
	phi := rotation * math.Pi / 180
	cos, sin := math.Cos(phi), math.Sin(phi)

	dx := (p0[0] - p1[0]) / 2
	dy := (p0[1] - p1[1]) / 2
	x1p := cos*dx + sin*dy
	y1p := -sin*dx + cos*dy

	rx2, ry2 := rx*rx, ry*ry
	lambda := x1p*x1p/rx2 + y1p*y1p/ry2
	if lambda > 1 {
		scale := math.Sqrt(lambda)
		rx *= scale
		ry *= scale
		rx2, ry2 = rx*rx, ry*ry
	}

	num := rx2*ry2 - rx2*y1p*y1p - ry2*x1p*x1p
	den := rx2*y1p*y1p + ry2*x1p*x1p
	if den == 0 {
		*out = append(*out, p1[0], p1[1])
		return
	}
	coef := math.Sqrt(math.Max(0, num/den))
	if large == sweep {
		coef = -coef
	}
	cxp := coef * rx * y1p / ry
	cyp := -coef * ry * x1p / rx
	centerX := cos*cxp - sin*cyp + (p0[0]+p1[0])/2
	centerY := sin*cxp + cos*cyp + (p0[1]+p1[1])/2

	ux := (x1p - cxp) / rx
	uy := (y1p - cyp) / ry
	vx := (-x1p - cxp) / rx
	vy := (-y1p - cyp) / ry
	theta1 := math.Atan2(uy, ux)
	dtheta := math.Atan2(ux*vy-uy*vx, ux*vx+uy*vy)
	if !sweep && dtheta > 0 {
		dtheta -= 2 * math.Pi
	} else if sweep && dtheta < 0 {
		dtheta += 2 * math.Pi
	}

	n := int(math.Ceil(math.Abs(dtheta) / (math.Pi / 2)))
	if n < 1 {
		n = 1
	}
	delta := dtheta / float64(n)
	k := 4.0 / 3.0 * math.Tan(delta/4)
	for i := 0; i < n; i++ {
		t1 := theta1 + delta*float64(i)
		t2 := t1 + delta
		q0 := ellipsePoint(centerX, centerY, rx, ry, phi, t1)
		q1 := ellipsePoint(centerX, centerY, rx, ry, phi, t2)
		d0 := ellipseDerivative(rx, ry, phi, t1)
		d1 := ellipseDerivative(rx, ry, phi, t2)
		cp1 := [2]float64{q0[0] + k*d0[0], q0[1] + k*d0[1]}
		cp2 := [2]float64{q1[0] - k*d1[0], q1[1] - k*d1[1]}
		flattenCubic(out, q0, cp1, cp2, q1, 0.35, 0)
	}
	if len(*out) >= 2 {
		(*out)[len(*out)-2] = p1[0]
		(*out)[len(*out)-1] = p1[1]
	}
}

func ellipsePoint(cx, cy, rx, ry, phi, t float64) [2]float64 {
	cos, sin := math.Cos(phi), math.Sin(phi)
	x, y := rx*math.Cos(t), ry*math.Sin(t)
	return [2]float64{cos*x - sin*y + cx, sin*x + cos*y + cy}
}

func ellipseDerivative(rx, ry, phi, t float64) [2]float64 {
	cos, sin := math.Cos(phi), math.Sin(phi)
	x, y := -rx*math.Sin(t), ry*math.Cos(t)
	return [2]float64{cos*x - sin*y, sin*x + cos*y}
}

type pathDParser struct {
	s string
	i int
}

func (p *pathDParser) eof() bool { return p.i >= len(p.s) }

func (p *pathDParser) skip() {
	for !p.eof() {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r', ',':
			p.i++
		default:
			return
		}
	}
}

func (p *pathDParser) hasNumber() bool {
	if p.eof() {
		return false
	}
	c := p.s[p.i]
	return c == '+' || c == '-' || c == '.' || (c >= '0' && c <= '9')
}

func (p *pathDParser) nextIsNumber() bool {
	p.skip()
	return p.hasNumber()
}

func (p *pathDParser) number() (float64, error) {
	p.skip()
	start := p.i
	if p.eof() {
		return 0, fmt.Errorf("d: expected number at end")
	}
	if p.s[p.i] == '+' || p.s[p.i] == '-' {
		p.i++
	}
	digits := false
	for !p.eof() && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
		p.i++
		digits = true
	}
	if !p.eof() && p.s[p.i] == '.' {
		p.i++
		for !p.eof() && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
			digits = true
		}
	}
	if !digits {
		return 0, fmt.Errorf("d: expected number at byte %d", start)
	}
	if !p.eof() && (p.s[p.i] == 'e' || p.s[p.i] == 'E') {
		p.i++
		if !p.eof() && (p.s[p.i] == '+' || p.s[p.i] == '-') {
			p.i++
		}
		exp := false
		for !p.eof() && p.s[p.i] >= '0' && p.s[p.i] <= '9' {
			p.i++
			exp = true
		}
		if !exp {
			return 0, fmt.Errorf("d: bad exponent at byte %d", start)
		}
	}
	v, err := strconv.ParseFloat(p.s[start:p.i], 64)
	if err != nil {
		return 0, fmt.Errorf("d: bad number %q", p.s[start:p.i])
	}
	return v, nil
}

func (p *pathDParser) flag() (bool, error) {
	p.skip()
	if p.eof() {
		return false, fmt.Errorf("d: expected arc flag at end")
	}
	switch p.s[p.i] {
	case '0':
		p.i++
		return false, nil
	case '1':
		p.i++
		return true, nil
	}
	return false, fmt.Errorf("d: bad arc flag %q at byte %d", p.s[p.i], p.i)
}

func isPathCommand(c byte) bool {
	switch c {
	case 'M', 'm', 'L', 'l', 'H', 'h', 'V', 'v',
		'C', 'c', 'S', 's', 'Q', 'q', 'T', 't',
		'A', 'a', 'Z', 'z':
		return true
	}
	return false
}

func flattenCubic(out *[]float64, p0, p1, p2, p3 [2]float64, tol float64, depth int) {
	if depth >= 12 || cubicFlat(p0, p1, p2, p3, tol) {
		*out = append(*out, p3[0], p3[1])
		return
	}
	p01 := midPoint(p0, p1)
	p12 := midPoint(p1, p2)
	p23 := midPoint(p2, p3)
	p012 := midPoint(p01, p12)
	p123 := midPoint(p12, p23)
	p0123 := midPoint(p012, p123)
	flattenCubic(out, p0, p01, p012, p0123, tol, depth+1)
	flattenCubic(out, p0123, p123, p23, p3, tol, depth+1)
}

func midPoint(a, b [2]float64) [2]float64 {
	return [2]float64{(a[0] + b[0]) / 2, (a[1] + b[1]) / 2}
}

// cubicFlat reports whether both control points sit within tol of the
// chord. Degenerate chords fall back to distance from p0.
func cubicFlat(p0, p1, p2, p3 [2]float64, tol float64) bool {
	dx, dy := p3[0]-p0[0], p3[1]-p0[1]
	den := dx*dx + dy*dy
	if den < 1e-9 {
		return dist2(p1, p0) <= tol*tol && dist2(p2, p0) <= tol*tol
	}
	near := func(p [2]float64) bool {
		// |cross| / |chord| is the perpendicular distance.
		cross := dx*(p[1]-p0[1]) - dy*(p[0]-p0[0])
		return (cross*cross)/den <= tol*tol
	}
	return near(p1) && near(p2)
}

func dist2(a, b [2]float64) float64 {
	dx, dy := a[0]-b[0], a[1]-b[1]
	return dx*dx + dy*dy
}

// pathDBounds returns the bbox of all flattened contours.
func pathDBounds(contours []pathContour) (minX, minY, maxX, maxY float64, ok bool) {
	for _, c := range contours {
		for i := 0; i+1 < len(c.points); i += 2 {
			x, y := c.points[i], c.points[i+1]
			if !ok {
				minX, minY, maxX, maxY = x, y, x, y
				ok = true
				continue
			}
			minX = math.Min(minX, x)
			minY = math.Min(minY, y)
			maxX = math.Max(maxX, x)
			maxY = math.Max(maxY, y)
		}
	}
	return
}
