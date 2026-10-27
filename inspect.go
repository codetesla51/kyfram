package kyfram

import (
	"fmt"
	"io"
	"sort"
)

// Inspect prints sampled entity state and box overlaps as plain text. It
// runs no renderer and needs no ffmpeg; Build already validated the
// script, so this is a pure interpolation walk.
func Inspect(tl Timeline, step float64, out io.Writer) error {
	if step <= 0 {
		return fmt.Errorf("step must be positive")
	}
	w, h := tl.size()
	duration := tl.Duration
	fmt.Fprintf(out, "canvas %dx%d  duration %.3fs  step %gs\n", w, h, duration, step)

	for _, t := range sampleTimes(0, duration, step) {
		v := viewAt(tl, t)
		fmt.Fprintf(out, "\nt=%s\n", fmtTime(t))

		type item struct {
			name string
			kind string
			text string
			box  bbox
			ok   bool
		}
		items := make([]item, 0, len(v.entities))
		for _, e := range v.entities {
			p := placeEntity(tl, v, e, t)
			alpha := propNum(p.props, "alpha", 1) * p.gAlpha
			if alpha <= 0 {
				continue
			}
			box, ok := p.bbox(tl)
			items = append(items, item{
				name: e.Name,
				kind: e.Kind,
				text: p.describe(box, ok, alpha),
				box:  box,
				ok:   ok,
			})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })
		for _, it := range items {
			fmt.Fprintf(out, "  %-16s %-10s %s\n", it.name, it.kind, it.text)
		}
		for i := 0; i < len(items); i++ {
			for j := i + 1; j < len(items); j++ {
				if items[i].ok && items[j].ok && items[i].box.intersects(items[j].box) {
					fmt.Fprintf(out, "OVERLAP %s/%s  %s / %s\n",
						items[i].name, items[j].name, items[i].box, items[j].box)
				}
			}
		}
	}
	return nil
}

// describe renders the resolved position/size fields plus the style
// columns shared by every kind.
func (p placedEntity) describe(b bbox, ok bool, alpha float64) string {
	x := propNum(p.props, "x", 0) + p.gx
	y := propNum(p.props, "y", 0) + p.gy
	var size string
	switch p.entity.Kind {
	case "circle", "star", "arc":
		size = fmt.Sprintf("x=%g y=%g r=%g", x, y, propNum(p.props, "r", 50))
	case "rect", "roundRect":
		size = fmt.Sprintf("x=%g y=%g w=%g h=%g", x, y,
			propNum(p.props, "w", 100), propNum(p.props, "h", 100))
	case "ellipse":
		rx := propNum(p.props, "rx", 50)
		size = fmt.Sprintf("x=%g y=%g rx=%g ry=%g", x, y, rx, propNum(p.props, "ry", rx))
	case "line":
		size = fmt.Sprintf("x1=%g y1=%g x2=%g y2=%g",
			propNum(p.props, "x1", 0)+p.gx, propNum(p.props, "y1", 0)+p.gy,
			propNum(p.props, "x2", 0)+p.gx, propNum(p.props, "y2", 0)+p.gy)
	case "text":
		size = fmt.Sprintf("x=%g y=%g size=%g", x, y, propNum(p.props, "size", 28))
	case "wave":
		size = fmt.Sprintf("x=%g y=%g width=%g amplitude=%g", x, y,
			propNum(p.props, "width", 640), propNum(p.props, "amplitude", 30))
	case "image", "path", "polygon":
		if ok {
			size = fmt.Sprintf("x=%g y=%g w=%g h=%g",
				(b.minX+b.maxX)/2, (b.minY+b.maxY)/2, b.maxX-b.minX, b.maxY-b.minY)
		} else {
			size = "no geometry"
		}
	default:
		size = fmt.Sprintf("x=%g y=%g", x, y)
	}
	color := propStr(p.props, "color", "#ffffff")
	if p.entity.Kind == "image" {
		color = "-"
	}
	group := p.group
	if group == "" {
		group = "-"
	}
	return fmt.Sprintf("%s color=%s alpha=%g group=%s", size, color, alpha, group)
}
