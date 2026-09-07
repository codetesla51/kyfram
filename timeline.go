package kyfram

// Timeline is the full scene description: how long the video runs,
// at what frame rate, and every animated entity in it. Width/Height
// of zero mean the default canvas (CLI flags or scene defaults win).
type Timeline struct {
	Duration float64
	FPS      int
	Width    int
	Height   int
	Entities []Entity
}

// Entity is one drawable thing (circle, rect, text...). Kind names
// the renderer dispatch; Keyframes must be sorted by Time.
type Entity struct {
	Name      string
	Kind      string
	Keyframes []Keyframe
}

// Keyframe fixes an entity's properties at one moment in seconds.
// Props holds numbers (float64), colors (hex string), or discrete
// values (string) — interpretation lives in the interpolator.
// Ease names the curve of the segment starting here
// (linear, easeIn, easeOut, easeInOut); empty means linear.
type Keyframe struct {
	Time  float64
	Props map[string]any
	Ease  string
}

// size resolves the render canvas: explicit dims win, zero means default.
func (tl Timeline) size() (int, int) {
	w, h := tl.Width, tl.Height
	if w <= 0 {
		w = frameW
	}
	if h <= 0 {
		h = frameH
	}
	return w, h
}
