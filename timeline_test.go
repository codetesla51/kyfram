package kyfram

import "testing"

// Hand-built Timeline must hold and return exactly what was put in.
func TestTimelineLiteral(t *testing.T) {
	tl := Timeline{
		Duration: 2,
		FPS:      30,
		Entities: []Entity{
			{
				Name: "ball",
				Kind: "circle",
				Keyframes: []Keyframe{
					{Time: 0, Props: map[string]any{"x": 0.0}},
					{Time: 2, Props: map[string]any{"x": 100.0}},
				},
			},
		},
	}

	if tl.Duration != 2 || tl.FPS != 30 {
		t.Fatalf("header = %+v, want duration 2 fps 30", tl)
	}
	got := tl.Entities[0].Keyframes[1].Props["x"]
	if got != 100.0 {
		t.Fatalf("x at t=2 = %v, want 100", got)
	}
}
