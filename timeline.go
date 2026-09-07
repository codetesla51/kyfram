package main

// Timeline is the full scene description: how long the video runs,
// at what frame rate, and every animated entity in it.
type Timeline struct {
	Duration float64
	FPS      int
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
type Keyframe struct {
	Time  float64
	Props map[string]any
}
