package kyfram

import (
	"strings"
	"testing"
)

func TestParsePathD(t *testing.T) {
	tests := []struct {
		name       string
		d          string
		contours   int
		closed     []bool
		firstPoint [2]float64
		lastPoint  [2]float64
		wantErr    string
	}{
		{
			name:       "move line close",
			d:          "M 0 0 L 10 0 L 10 10 Z",
			contours:   1,
			closed:     []bool{true},
			firstPoint: [2]float64{0, 0},
			lastPoint:  [2]float64{10, 10},
		},
		{
			name:       "cubic",
			d:          "M0,0 C0,10 10,10 10,0",
			contours:   1,
			closed:     []bool{false},
			firstPoint: [2]float64{0, 0},
			lastPoint:  [2]float64{10, 0},
		},
		{
			name:       "two subpaths",
			d:          "M0 0 L10 0 Z M20 20 L30 20 Z",
			contours:   2,
			closed:     []bool{true, true},
			firstPoint: [2]float64{0, 0},
			lastPoint:  [2]float64{30, 20},
		},
		{
			name:       "relative",
			d:          "m10 10 l10 0",
			contours:   1,
			closed:     []bool{false},
			firstPoint: [2]float64{10, 10},
			lastPoint:  [2]float64{20, 10},
		},
		{
			name:       "implicit line after move",
			d:          "M 0 0 10 0 10 10",
			contours:   1,
			closed:     []bool{false},
			firstPoint: [2]float64{0, 0},
			lastPoint:  [2]float64{10, 10},
		},
		{
			name:    "missing y",
			d:       "M 0",
			wantErr: "expected number",
		},
		{
			name:    "line before move",
			d:       "L 10 10",
			wantErr: "before M",
		},
		{
			name:    "short cubic",
			d:       "M 0 0 C 1 2 3 4",
			wantErr: "expected number",
		},
		{
			name:    "unknown command",
			d:       "M 0 0 X 1 2",
			wantErr: "unexpected",
		},
		{
			name:    "empty",
			d:       "",
			wantErr: "no drawable path",
		},
		{
			name:    "bare line",
			d:       "M 0 0 L",
			wantErr: "L needs coordinates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contours, err := parsePathD(tt.d)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePathD: %v", err)
			}
			if len(contours) != tt.contours {
				t.Fatalf("contours = %d, want %d", len(contours), tt.contours)
			}
			for i, want := range tt.closed {
				if contours[i].closed != want {
					t.Fatalf("contour %d closed = %v, want %v", i, contours[i].closed, want)
				}
			}
			first := contours[0].points
			if first[0] != tt.firstPoint[0] || first[1] != tt.firstPoint[1] {
				t.Fatalf("first point = %v, want %v", first[:2], tt.firstPoint)
			}
			last := contours[len(contours)-1].points
			n := len(last)
			if last[n-2] != tt.lastPoint[0] || last[n-1] != tt.lastPoint[1] {
				t.Fatalf("last point = %v, want %v", last[n-2:], tt.lastPoint)
			}
		})
	}
}

func TestRenderPathD(t *testing.T) {
	tl := Timeline{
		Duration: 1,
		FPS:      30,
		Entities: []Entity{
			{
				Name: "curve",
				Kind: "path",
				Keyframes: []Keyframe{{
					Time: 0,
					Props: map[string]any{
						"d":     "M 20 180 C 160 40 320 40 460 180",
						"width": 10.0,
						"color": "#ffffff",
						"cap":   "round",
					},
				}},
			},
		},
	}
	img := RenderFrame(tl, 0).Image()
	// Cubic midpoint: x=240, y=75. Allow a few pixels for flattening
	// and antialiasing.
	found := false
	for dy := -4; dy <= 4; dy++ {
		r, g, b, _ := img.At(240, 75+dy).RGBA()
		if r>>8 > 0xf0 && g>>8 > 0xf0 && b>>8 > 0xf0 {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("cubic d path midpoint was not painted")
	}
}

func TestRenderPathDTwoSubpaths(t *testing.T) {
	tl := Timeline{
		Duration: 1,
		FPS:      30,
		Entities: []Entity{
			{
				Name: "lines",
				Kind: "path",
				Keyframes: []Keyframe{{
					Time: 0,
					Props: map[string]any{
						"d":     "M 20 100 L 120 100 M 20 200 L 120 200",
						"width": 8.0,
						"color": "#ffffff",
					},
				}},
			},
		},
	}
	img := RenderFrame(tl, 0).Image()
	for _, y := range []int{100, 200} {
		r, _, _, _ := img.At(70, y).RGBA()
		if r>>8 != 0xff {
			t.Fatalf("subpath at y=%d was not painted", y)
		}
	}
}

func TestValidatePathD(t *testing.T) {
	err := buildErr(t, `
scene(0.1, 30)
shape("curve", "path", table{
  "0": table{d: "M 0 0 C 1 2 3 4", width: 2, color: "#ffffff"},
})
`)
	if !strings.Contains(err, "invalid d") {
		t.Fatalf("error = %q, want invalid d", err)
	}
}

func TestValidatePathDBoth(t *testing.T) {
	err := buildErr(t, `
scene(0.1, 30)
shape("curve", "path", table{
  "0": table{d: "M 0 0 L 10 10", points: [0, 0, 10, 10], width: 2, color: "#ffffff"},
})
`)
	if !strings.Contains(err, `both "d" and "points"`) {
		t.Fatalf("error = %q, want both d and points", err)
	}
}
