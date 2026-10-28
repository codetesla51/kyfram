package kyfram

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codetesla51/logos/interpreter"
	"github.com/fogleman/gg"
)

func TestParseSVGShapesAndTransforms(t *testing.T) {
	src := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="100" height="50" viewBox="0 0 200 100">
  <defs>
    <linearGradient id="g" x1="0%" y1="0%" x2="100%" y2="0%">
      <stop offset="0%" stop-color="#ff0000"/>
      <stop offset="100%" stop-color="#0000ff"/>
    </linearGradient>
  </defs>
  <g transform="translate(10 5) scale(2)" fill="url(#g)">
    <rect x="1" y="2" width="30" height="20" rx="3"/>
    <path d="M0 0 H20 V10 A 5 5 0 0 1 0 10 Z" fill="none" stroke="#00ff00" stroke-width="2" stroke-linecap="round"/>
  </g>
</svg>`)
	doc, err := parseSVG(src)
	if err != nil {
		t.Fatalf("parseSVG: %v", err)
	}
	if doc.Width != 100 || doc.Height != 50 {
		t.Fatalf("size = %v x %v, want 100 x 50", doc.Width, doc.Height)
	}
	if len(doc.Shapes) != 2 {
		t.Fatalf("shapes = %d, want 2", len(doc.Shapes))
	}
	if doc.Shapes[0].Fill != "url(#g)" {
		t.Fatalf("fill = %q, want gradient", doc.Shapes[0].Fill)
	}
	if len(doc.Shapes[0].Contours) != 1 || len(doc.Shapes[0].Contours[0].points) < 8 {
		t.Fatalf("rect contours = %#v", doc.Shapes[0].Contours)
	}
	box, ok := svgContoursBox(doc.Shapes[0].Contours)
	if !ok {
		t.Fatal("rect has no box")
	}
	// translate(10 5) scale(2) maps (1,2) to (12,9) and the 30x20
	// rect to 60x40.
	if math.Abs(box.X-12) > 0.01 || math.Abs(box.Y-9) > 0.01 ||
		math.Abs(box.W-60) > 0.01 || math.Abs(box.H-40) > 0.01 {
		t.Fatalf("rect box = %+v, want x=12 y=9 w=60 h=40", box)
	}
	if doc.Shapes[1].Fill != "none" || doc.Shapes[1].Stroke != "#00ff00" {
		t.Fatalf("path paints = fill %q stroke %q", doc.Shapes[1].Fill, doc.Shapes[1].Stroke)
	}
	if math.Abs(doc.Shapes[1].StrokeWidth-2) > 0.01 || math.Abs(doc.Shapes[1].StrokeScale-2) > 0.01 {
		t.Fatalf("stroke width=%v scale=%v", doc.Shapes[1].StrokeWidth, doc.Shapes[1].StrokeScale)
	}
}

func TestSVGRenderSolidFill(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "red.svg")
	if err := os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><rect width="10" height="10" fill="#ff0000"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := loadSVG(path)
	if err != nil {
		t.Fatal(err)
	}
	dc := gg.NewContext(100, 100)
	drawSVGDoc(dc, doc, 0, 0, 100, 100, 1)
	got := color.NRGBAModel.Convert(dc.Image().At(50, 50)).(color.NRGBA)
	want := color.NRGBA{R: 255, A: 255}
	if got != want {
		t.Fatalf("center = %#v, want %#v", got, want)
	}
}

func TestSVGRenderViewBoxAndAspect(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "wide.svg")
	// A 20x10 viewBox placed into a 100x100 box with the default
	// xMidYMid meet: scale is 5, so the 20x10 drawing occupies the
	// middle 100x50 strip.
	if err := os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 10"><rect width="20" height="10" fill="#00ff00"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	doc, err := loadSVG(path)
	if err != nil {
		t.Fatal(err)
	}
	dc := gg.NewContext(100, 100)
	drawSVGDoc(dc, doc, 0, 0, 100, 100, 1)
	var inside, outside int
	for y := 0; y < 100; y++ {
		c := color.NRGBAModel.Convert(dc.Image().At(50, y)).(color.NRGBA)
		if c.G > 0 {
			inside++
		} else {
			outside++
		}
	}
	if inside < 48 || inside > 52 || outside < 48 || outside > 52 {
		t.Fatalf("green rows=%d outside=%d, want about 50/50", inside, outside)
	}
}

func TestSVGGradientPaint(t *testing.T) {
	src := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">
	  <defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="0">
	    <stop offset="0" stop-color="#ff0000"/>
	    <stop offset="1" stop-color="#0000ff"/>
	  </linearGradient></defs>
	  <rect width="10" height="10" fill="url(#g)"/>
	</svg>`)
	doc, err := parseSVG(src)
	if err != nil {
		t.Fatal(err)
	}
	dc := gg.NewContext(100, 100)
	drawSVGDoc(dc, doc, 0, 0, 100, 100, 1)
	left := color.NRGBAModel.Convert(dc.Image().At(5, 50)).(color.NRGBA)
	right := color.NRGBAModel.Convert(dc.Image().At(95, 50)).(color.NRGBA)
	if left.R < 200 || left.B > 60 {
		t.Fatalf("left = %#v, want red", left)
	}
	if right.B < 200 || right.R > 60 {
		t.Fatalf("right = %#v, want blue", right)
	}
}

func TestBindingsSVG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mark.svg")
	if err := os.WriteFile(path, []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><circle cx="5" cy="5" r="5" fill="#ff0000"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `
scene(1, 30)
svg("mark", "` + path + `", table{
  "0": table{x: 320, y: 180, w: 80, h: 80},
})
`
	vm := interpreter.NewInterpreter(interpreter.SandboxConfig{})
	var b Builder
	b.Bind(vm)
	if err := vm.Run(src); err != nil {
		t.Fatalf("Run: %v", err)
	}
	tl, err := b.Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	ents := tl.Scenes[0].Entities
	if len(ents) != 1 || ents[0].Kind != "svg" {
		t.Fatalf("entities = %+v", ents)
	}
	if got := ents[0].Keyframes[0].Props["path"]; got != path {
		t.Fatalf("path = %v, want %q", got, path)
	}
}

func TestBuildBadSVG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.svg")
	if err := os.WriteFile(path, []byte(`<svg><path d="M0 0 L"/></svg>`), 0o644); err != nil {
		t.Fatal(err)
	}
	src := `
scene(1, 30)
svg("mark", "` + path + `", table{
  "0": table{x: 320, y: 180},
})
`
	vm := interpreter.NewInterpreter(interpreter.SandboxConfig{})
	var b Builder
	b.Bind(vm)
	if err := vm.Run(src); err != nil {
		t.Fatalf("Run: %v", err)
	}
	_, err := b.Build()
	if err == nil || !strings.Contains(err.Error(), "svg") {
		t.Fatalf("Build error = %v, want svg parse error", err)
	}
}

func TestSVGArcGeometry(t *testing.T) {
	for _, tc := range []struct {
		d    string
		want svgBox
	}{
		{"M0 0 A 10 10 0 0 1 20 0", svgBox{X: 0, Y: -10, W: 20, H: 10}},
		{"M0 0 A 10 10 0 0 0 20 0", svgBox{X: 0, Y: 0, W: 20, H: 10}},
	} {
		cs, err := parsePathD(tc.d)
		if err != nil {
			t.Fatalf("%s: %v", tc.d, err)
		}
		got, ok := svgContoursBox(cs)
		if !ok {
			t.Fatalf("%s: no box", tc.d)
		}
		if math.Abs(got.X-tc.want.X) > 0.2 || math.Abs(got.Y-tc.want.Y) > 0.2 ||
			math.Abs(got.W-tc.want.W) > 0.2 || math.Abs(got.H-tc.want.H) > 0.2 {
			t.Fatalf("%s: box = %+v, want %+v", tc.d, got, tc.want)
		}
	}
}

func TestSVGUseReference(t *testing.T) {
	src := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10">
	  <defs><path id="tile" d="M0 0 H10 V10 Z" fill="#ff0000"/></defs>
	  <use href="#tile" x="1" y="2"/>
	</svg>`)
	doc, err := parseSVG(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 1 {
		t.Fatalf("shapes = %d, want 1", len(doc.Shapes))
	}
	box, ok := svgContoursBox(doc.Shapes[0].Contours)
	if !ok {
		t.Fatal("no box")
	}
	if math.Abs(box.X-1) > 0.01 || math.Abs(box.Y-2) > 0.01 {
		t.Fatalf("box = %+v, want x=1 y=2", box)
	}
}

func TestSVGUseDoesNotMutatePathCache(t *testing.T) {
	src := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 20">
	  <defs><path id="box" d="M0 0 H10 V10 Z"/></defs>
	  <use href="#box" transform="translate(10 0)"/>
	  <use href="#box" transform="translate(20 0)"/>
	</svg>`)
	doc, err := parseSVG(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Shapes) != 2 {
		t.Fatalf("shapes = %d, want 2", len(doc.Shapes))
	}
	for i, wantX := range []float64{10, 20} {
		box, ok := svgContoursBox(doc.Shapes[i].Contours)
		if !ok {
			t.Fatalf("shape %d has no box", i)
		}
		if math.Abs(box.X-wantX) > 0.01 {
			t.Fatalf("shape %d x = %v, want %v", i, box.X, wantX)
		}
	}
}
