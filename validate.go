package kyfram

import (
	"fmt"
	"sort"
)

// knownKinds is every kind drawEntity understands.
var knownKinds = []string{
	"circle", "text", "rect", "line", "arc", "image", "svg", "wave",
	"path", "polygon", "roundRect", "star", "ellipse",
}

// knownProps is every prop the renderer can read. A script prop outside
// this set is a typo the renderer would silently ignore (defaulting),
// so Build rejects it with a suggestion.
var knownProps = []string{
	"alpha", "amplitude", "blend", "blur", "cap", "clip", "clipD",
	"clipH", "clipPoints", "clipR", "clipRadius", "clipW", "clipX",
	"clipY", "closed", "color", "content", "d", "end", "filled", "font",
	"frequency", "glow", "glowAlpha", "glowBlur", "gradAngle", "gradFrom",
	"gradTo", "h", "inner", "join", "layer", "lineHeight", "path", "phase",
	"points", "r", "radius", "rotate", "rx", "ry", "scale", "shadowAlpha",
	"shadowBlur", "shadowColor", "shadowX", "shadowY", "size", "start",
	"thickness", "trim", "trimStart", "w", "width", "wrap",
	"x", "x1", "x2", "y", "y1", "y2",
}

// numProps must hold numbers; a wrong-typed value would silently fall
// back to the renderer default (e.g. x: "left" draws at center).
var numProps = []string{
	"alpha", "amplitude", "blur", "clipH", "clipR", "clipRadius", "clipW",
	"clipX", "clipY", "end", "frequency", "glowAlpha", "glowBlur",
	"gradAngle", "h", "inner", "layer", "lineHeight", "phase", "r",
	"radius", "rotate", "rx", "ry", "scale", "shadowAlpha", "shadowBlur",
	"shadowX", "shadowY", "size", "start", "thickness", "trim", "trimStart",
	"w", "width", "wrap", "x", "x1", "x2", "y", "y1", "y2",
}

// strProps must hold strings. font/path get existence checks below;
// color/gradFrom/gradTo must additionally parse as colors.
var strProps = []string{
	"blend", "cap", "clip", "clipD", "color", "content", "d", "font",
	"glow", "gradFrom", "gradTo", "join", "path", "shadowColor",
}

// colorProps must parse via parseColor (hex or named); anything else
// silently snaps to a default instead of blending.
var colorProps = []string{"color", "glow", "gradFrom", "gradTo", "shadowColor"}

// enumProps lists the accepted values for string enums the renderer
// otherwise degrades to a default without a word.
var enumProps = map[string][]string{
	"blend": {"add", "multiply", "screen"},
	"cap":   {"butt", "round", "square"},
	"clip":  {"circle", "none", "path", "rect", "roundRect"},
	"join":  {"bevel", "miter", "round"},
}

// colorNameList is the sorted named-color vocabulary for suggestions.
func colorNameList() []string {
	names := make([]string, 0, len(colorNames))
	for n := range colorNames {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// validate runs fail-fast checks over the declared timeline so a script
// mistake surfaces at build time, not mid-encode on frame 300.
func (b *Builder) validate() error {
	var allGroups []Group
	allGroups = append(allGroups, b.tl.Groups...)
	for _, sc := range b.tl.Scenes {
		allGroups = append(allGroups, sc.Groups...)
	}
	groups := make(map[string]bool, len(allGroups))
	for _, g := range allGroups {
		groups[g.Name] = true
	}
	groupParent := make(map[string]string, len(allGroups))
	for _, g := range allGroups {
		parent := ""
		for _, kf := range g.Keyframes {
			v, ok := kf.Props["parent"]
			if !ok {
				continue
			}
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("group %q: parent at t=%g must be a string", g.Name, kf.Time)
			}
			if parent != "" && parent != s {
				return fmt.Errorf("group %q: parent must stay constant (got %q and %q)", g.Name, parent, s)
			}
			parent = s
		}
		if parent != "" && !groups[parent] {
			return fmt.Errorf("group %q: parent %q does not exist", g.Name, parent)
		}
		groupParent[g.Name] = parent
	}
	for name := range groupParent {
		seen := map[string]bool{name: true}
		for p := groupParent[name]; p != ""; p = groupParent[p] {
			if seen[p] {
				return fmt.Errorf("group %q: nested group cycle", name)
			}
			seen[p] = true
		}
	}
	check := func(where string, e Entity) error {
		if !contains(knownKinds, e.Kind) {
			return fmt.Errorf("%s entity %q: unknown kind %q (want one of %v)%s",
				where, e.Name, e.Kind, knownKinds, suggest(knownKinds, e.Kind))
		}
		if e.Group != "" && !groups[e.Group] {
			return fmt.Errorf("%s entity %q: group %q does not exist (declare group(%q, ...) before use)",
				where, e.Name, e.Group, e.Group)
		}
		for _, kf := range e.Keyframes {
			for p, v := range kf.Props {
				if !contains(knownProps, p) {
					return fmt.Errorf("%s entity %q: unknown prop %q at t=%g%s",
						where, e.Name, p, kf.Time, suggest(knownProps, p))
				}
				if err := checkValue(where, e, kf.Time, p, v); err != nil {
					return err
				}
			}
		}
		if e.Kind == "path" {
			hasD, hasPoints := false, false
			for _, kf := range e.Keyframes {
				if _, ok := kf.Props["d"]; ok {
					hasD = true
				}
				if _, ok := kf.Props["points"]; ok {
					hasPoints = true
				}
			}
			if hasD && hasPoints {
				return fmt.Errorf("%s entity %q: path cannot set both \"d\" and \"points\"", where, e.Name)
			}
		}
		if e.Kind == "image" || e.Kind == "svg" {
			for _, kf := range e.Keyframes {
				p, ok := kf.Props["path"]
				if !ok {
					return fmt.Errorf("%s entity %q: %s needs a \"path\" prop at t=%g (without one it draws nothing)",
						where, e.Name, e.Kind, kf.Time)
				}
				// Type already proven string by checkValue above.
				if err := checkAsset(where, e.Name, kf.Time, e.Kind, p.(string)); err != nil {
					return err
				}
				if e.Kind == "svg" {
					if _, err := loadSVG(p.(string)); err != nil {
						return fmt.Errorf("%s entity %q: svg %q at t=%g: %v",
							where, e.Name, p.(string), kf.Time, err)
					}
				}
			}
		}
		return nil
	}
	for _, e := range b.tl.Entities {
		if err := check("global", e); err != nil {
			return err
		}
	}
	for si, sc := range b.tl.Scenes {
		where := fmt.Sprintf("scene %d", si+1)
		for _, e := range sc.Entities {
			if err := check(where, e); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkValue type-checks one prop value against what the renderer reads.
// The renderer degrades wrong types to defaults in silence, so Build
// rejects them here with the prop name and time.
func checkValue(where string, e Entity, t float64, p string, v any) error {
	bad := func(want string) error {
		return fmt.Errorf("%s entity %q: prop %q at t=%g must be %s", where, e.Name, p, t, want)
	}
	if contains(numProps, p) {
		if _, ok := toFloat(v); !ok {
			return bad("a number")
		}
		return nil
	}
	if p == "closed" || p == "filled" {
		if _, ok := v.(bool); !ok {
			return bad("a bool")
		}
		return nil
	}
	if p == "points" {
		// Star takes a numeric count; path/polygon/text take a number list.
		if _, ok := toFloat(v); ok {
			return nil
		}
		raw, ok := v.([]any)
		if !ok {
			return bad("a number or a number list")
		}
		for _, n := range raw {
			if _, ok := toFloat(n); !ok {
				return bad("a number or a number list")
			}
		}
		return nil
	}
	if p == "clipPoints" {
		raw, ok := v.([]any)
		if !ok {
			return bad("a number list")
		}
		for _, n := range raw {
			if _, ok := toFloat(n); !ok {
				return bad("a number list")
			}
		}
		return nil
	}
	if contains(strProps, p) {
		s, ok := v.(string)
		if !ok {
			return bad("a string")
		}
		if contains(colorProps, p) {
			if _, err := parseColor(s); err != nil {
				return fmt.Errorf("%s entity %q: invalid color %q at t=%g (want #rgb/#rrggbb or a named color)%s",
					where, e.Name, s, t, suggest(colorNameList(), s))
			}
		}
		if want, ok := enumProps[p]; ok {
			if !contains(want, s) {
				return fmt.Errorf("%s entity %q: invalid %s %q at t=%g (want one of %v)%s",
					where, e.Name, p, s, t, want, suggest(want, s))
			}
		}
		if p == "font" {
			if err := checkAsset(where, e.Name, t, "font", s); err != nil {
				return err
			}
		}
		if p == "d" || p == "clipD" {
			if _, err := flattenPathD(s); err != nil {
				return fmt.Errorf("%s entity %q: invalid %s at t=%g: %w", where, e.Name, p, t, err)
			}
		}
	}
	return nil
}

// checkAsset stats a font/image file the renderer would silently skip if
// missing. Paths resolve against the render working directory, same as
// the renderer sees them.
func checkAsset(where, name string, t float64, what, path string) error {
	if err := statAsset(path); err != nil {
		return fmt.Errorf("%s entity %q: %s file %q not found (t=%g)", where, name, what, path, t)
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
