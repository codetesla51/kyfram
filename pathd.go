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

// flattenPathD parses and flattens an SVG-style subset: M, L, C, Z
// (absolute or relative). Curves are flattened adaptively at render
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
	var cmd byte
	var haveCmd, moved bool

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
		// L/C continues from there in a fresh contour.
		cur = &pathContour{points: []float64{cx, cy}}
		return nil
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
				cur.points = append(cur.points, x, y)
				cx, cy = x, y
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
				cur.points = append(cur.points, x, y)
				cx, cy = x, y
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
			}

		case 'Z', 'z':
			return nil, fmt.Errorf("d: Z takes no coordinates")
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

func isPathCommand(c byte) bool {
	switch c {
	case 'M', 'm', 'L', 'l', 'C', 'c', 'Z', 'z':
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
