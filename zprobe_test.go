package kyfram

import (
	"fmt"
	"image"
	"testing"

	"github.com/fogleman/gg"
)

func TestZProbeFadeBranch(t *testing.T) {
	dc := gg.NewContext(1, 1)
	dc.SetRGB(0x10/255.0, 0x10/255.0, 0x18/255.0)
	dc.Clear()
	layer := gg.NewContext(1, 1)
	layer.SetRGB(0x18/255.0, 0x10/255.0, 0x10/255.0)
	layer.Clear()
	r, g, b, _ := dc.Image().At(0, 0).RGBA()
	fmt.Printf("before #%02x%02x%02x\n", r>>8, g>>8, b>>8)
	compositeFade(dc.Image().(*image.RGBA), layer.Image().(*image.RGBA), 0.3)
	r, g, b, _ = dc.Image().At(0, 0).RGBA()
	fmt.Printf("after #%02x%02x%02x want r~18 g~16 b~21\n", r>>8, g>>8, b>>8)
}
