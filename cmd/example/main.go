// Command example renders one frame of a hand-built Timeline to out.png.
// Throwaway until the real render CLI lands (step 7/10).
package main

import (
	"log"
	"os"

	kyfram "github.com/codetesla51/kyfram"
)

func main() {
	tl := kyfram.Timeline{
		Duration: 2,
		FPS:      30,
		Entities: []kyfram.Entity{
			{
				Name: "ball",
				Kind: "circle",
				Keyframes: []kyfram.Keyframe{
					{Time: 0, Props: map[string]any{"x": 100.0, "y": 160.0, "r": 80.0, "color": "#e64c40"}},
					{Time: 2, Props: map[string]any{"x": 320.0, "y": 160.0, "r": 80.0, "color": "#e64c40"}},
				},
			},
			{
				Name: "caption",
				Kind: "text",
				Keyframes: []kyfram.Keyframe{
					{Time: 0, Props: map[string]any{"content": "hello kyfram", "x": 320.0, "y": 300.0, "size": 28.0, "color": "#ffffff"}},
					{Time: 2, Props: map[string]any{"content": "hello kyfram", "x": 320.0, "y": 300.0, "size": 28.0, "color": "#ffffff"}},
				},
			},
		},
	}

	// With a directory argument, dump the whole PNG sequence there
	// (flip through to check motion); otherwise one frame to out.png.
	if len(os.Args) > 1 {
		if err := kyfram.RenderSequence(tl, os.Args[1]); err != nil {
			log.Fatalf("sequence: %v", err)
		}
		return
	}
	if err := kyfram.RenderFrame(tl, 2).SavePNG("out.png"); err != nil {
		log.Fatalf("save: %v", err)
	}
}
