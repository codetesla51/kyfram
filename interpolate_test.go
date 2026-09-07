package kyfram

import "testing"

// Numbers lerp, hex colors lerp per-channel, anything else snaps to
// the earlier keyframe. Past-the-end holds the last value.
func TestInterpolate(t *testing.T) {
	kfs := []Keyframe{
		{Time: 0, Props: map[string]any{"x": 0.0, "color": "#000000", "label": "a"}},
		{Time: 2, Props: map[string]any{"x": 100.0, "color": "#ffffff", "label": "b"}},
	}

	cases := []struct {
		name string
		kfs  []Keyframe
		t    float64
		key  string
		want any
	}{
		{"start", kfs, 0, "x", 0.0},
		{"mid number", kfs, 1, "x", 50.0},
		{"end number", kfs, 2, "x", 100.0},
		{"past end holds", kfs, 5, "x", 100.0},
		{"before start holds", kfs, -1, "x", 0.0},
		{"mid color", kfs, 1, "color", "#808080"},
		{"past end color", kfs, 9, "color", "#ffffff"},
		{"mid string snaps to earlier", kfs, 1, "label", "a"},
		{"end string", kfs, 2, "label", "b"},
		{"empty", nil, 1, "x", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Interpolate(c.kfs, c.t)[c.key]
			if got != c.want {
				t.Fatalf("Interpolate(t=%v)[%q] = %v, want %v", c.t, c.key, got, c.want)
			}
		})
	}
}
