package kyfram

// Timeline is the full scene description: how long the video runs,
// at what frame rate, and every animated entity in it. Width/Height
// of zero mean the default canvas (CLI flags or scene defaults win).
// Empty Font means the built-in default typeface.
//
// A script may declare one or more scenes; with scenes, each scene
// owns its entities/groups/camera (declared after its scene() call,
// times auto-offset to absolute) and Duration is the sum. Entities
// declared before any scene() are global and drawn in every scene.
type Timeline struct {
	Duration    float64
	FPS         int
	Width       int
	Height      int
	Font        string
	Transparent bool
	Entities    []Entity
	Groups      []Group
	Scenes      []Scene
	Audio       []AudioTrack
	// Camera holds x/y/zoom keyframes; empty means a locked frame.
	// Used directly when there are no scenes; scenes carry their own.
	Camera []Keyframe
}

// Scene is one segment of a multi-scene timeline. Duration is the
// scene's own length in seconds (declared keyframes use scene-local
// times, offset to absolute at bind). Transition names the effect at
// the scene's start ("fade", "to-black", "push-left", ...; empty or
// "cut" = instant) and TransitionDur its length in seconds; zero uses
// the mode's default.
type Scene struct {
	Name          string
	Duration      float64
	Entities      []Entity
	Groups        []Group
	Camera        []Keyframe
	Transition    string
	TransitionDur float64
}

// AudioTrack lays one sound file under the video at Offset seconds.
// Loop repeats short sounds to the last frame instead of going silent.
type AudioTrack struct {
	Path   string
	Offset float64
	Loop   bool
}

// Entity is one drawable thing (circle, rect, text...). Kind names
// the renderer dispatch; Keyframes must be sorted by Time.
type Entity struct {
	Name      string
	Kind      string
	Keyframes []Keyframe
	Group     string // optional group membership; empty means root
}

// Group is a transform container for children. Its keyframes hold
// x, y, scale, rotate, alpha, layer, and optional parent. Children are
// entities whose Group == Name; they keep local x/y and animate
// independently. A group with parent set nests inside that group, so
// world transform = parent * group * child.
type Group struct {
	Name      string
	Keyframes []Keyframe
}

// Keyframe fixes an entity's properties at one moment in seconds.
// Props holds numbers (float64), colors (hex string), or discrete
// values (string) — interpretation lives in the interpolator.
// Ease names the curve of the segment starting here (see easeNames);
// empty means linear.
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
