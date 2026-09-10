package kyfram

import (
	"fmt"
)

// knownKinds is every kind drawEntity understands.
var knownKinds = []string{
	"circle", "text", "rect", "line", "arc", "image", "wave",
	"path", "polygon", "roundRect", "star", "ellipse",
}

// knownProps is every prop the renderer can read. A script prop outside
// this set is a typo the renderer would silently ignore (defaulting),
// so Build rejects it with a suggestion.
var knownProps = []string{
	"alpha", "amplitude", "blend", "cap", "closed", "color", "content",
	"end", "filled", "font", "frequency", "gradAngle", "gradFrom",
	"gradTo", "h", "inner", "join", "layer", "path", "phase", "points",
	"r", "radius", "rotate", "rx", "ry", "scale", "size", "start",
	"thickness", "w", "width", "wrap", "x", "x1", "x2", "y", "y1", "y2",
}

// validate runs fail-fast checks over the declared timeline so a script
// mistake surfaces at build time, not mid-encode on frame 300.
func (b *Builder) validate() error {
	groups := make(map[string]bool, len(b.tl.Groups))
	for _, g := range b.tl.Groups {
		groups[g.Name] = true
	}
	for _, e := range b.tl.Entities {
		if !contains(knownKinds, e.Kind) {
			return fmt.Errorf("entity %q: unknown kind %q (want one of %v)%s",
				e.Name, e.Kind, knownKinds, suggest(knownKinds, e.Kind))
		}
		if e.Group != "" && !groups[e.Group] {
			return fmt.Errorf("entity %q: group %q does not exist (declare group(%q, ...) before use)",
				e.Name, e.Group, e.Group)
		}
		for _, kf := range e.Keyframes {
			for p := range kf.Props {
				if !contains(knownProps, p) {
					return fmt.Errorf("entity %q: unknown prop %q at t=%g%s",
						e.Name, p, kf.Time, suggest(knownProps, p))
				}
			}
		}
	}
	return nil
}

func contains(known []string, got string) bool {
	for _, k := range known {
		if k == got {
			return true
		}
	}
	return false
}

// suggest returns "; did you mean %q?" for the closest known label when
// it's within edit-distance 2, otherwise "".
func suggest(known []string, got string) string {
	best, bestD := "", 99
	for _, k := range known {
		if d := editDistance(k, got); d < bestD {
			best, bestD = k, d
		}
	}
	if bestD <= 2 {
		return fmt.Sprintf("; did you mean %q?", best)
	}
	return ""
}

// editDistance is Levenshtein distance, plenty for short label typos.
func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := range lb + 1 {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(min(cur[j-1]+1, prev[j]+1), prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}
