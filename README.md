# kyfram

> A declarative, keyframe-driven video engine where **the video is written in
> Logos** and Go is only the runtime underneath.

`kyfram` is a small, opinionated engine for building motion-graphics clips.
You declare what shapes look like at specific times in a `.lgv` script —
Go interpolates between keyframes, rasterizes each frame in memory, and
pipes raw pixels to `ffmpeg` for encoding. Same lineage as `logos2d`
(sibling repo, live 2D games); this one is batch video instead of live play.

The mental model is CSS `@keyframes` applied to video files. Remotion,
Motion Canvas, and Manim are all imperative (sequences of commands).
Here you declare states-at-times and the engine owns 100% of the
sequencing and tweening — retiming is editing a number, not re-directing.

## Quick start

```sh
# Prerequisites: Go 1.27+, system ffmpeg, a TrueType font at
# /usr/share/fonts/noto/NotoSans-Regular.ttf (override per text via font:)

go build ./...                                   # typechecks everything
go run ./cmd/kyfram render examples/intro.lgv    # → out.mp4
go run ./cmd/kyfram render --out v.mp4 --fps 15 --width 320 --height 180 examples/intro.lgv
go run ./cmd/kyfram view examples/intro.lgv      # → http://localhost:8080, scrub + play + hot reload
```

A minimal script (`examples/intro.lgv`):

```js
scene(2, 30)

shape("ball", "circle", table{
  "0": table{x: 100, y: 160, r: 80, color: "#e64c40"},
  "2": table{x: 320, y: 160, r: 80, color: "#e64c40"}
})

text("caption", table{
  "0": table{content: "hello kyfram", x: 320, y: 300, size: 28, color: "#ffffff"},
  "2": table{content: "hello kyfram", x: 320, y: 300, size: 28, color: "#ffffff"}
})
```

## Features

- **Declarative keyframes** — `scene()` / `shape()` / `text()` / `audio()` builtins; scripts compute freely, the builder keeps only declarations
- **Interpolator** — numbers lerp, hex colors lerp per channel, strings snap to the earlier key; out-of-range holds the edge, never extrapolates
- **Easing** — `ease: "linear" | "easeIn" | "easeOut" | "easeInOut"` per segment, declared on the earlier key
- **Primitives** — circle, rect, line, image, font-backed text
- **Zero-disk render path** — raw RGBA bytes piped to ffmpeg's stdin (`-f rawvideo`); PNG sequences exist only as a debug path
- **Audio mux** — `audio(path, offset?)` tracks mixed under the video, beeps land on visual beats via `-itsoffset`
- **Preview server** — scrub slider, play button, hot reload (every request re-reads the script)
- **Sandboxed scripts** — file IO, network, shell, and exit are blocked; "declarations only" is convention, the builder ignores the rest

## Examples

| script | shows |
|---|---|
| `examples/intro.lgv` | ball glide + caption (start here) |
| `examples/showcase.lgv` | 3-key arc, color washes, fading beam |
| `examples/rect.lgv` | sliding, recoloring rect |
| `examples/line.lgv` | sweeping line |
| `examples/image.lgv` | image asset gliding (`assets/ball.png`) |
| `examples/text.lgv` | growing text + snapping content |
| `examples/ease.lgv` | linear vs `easeIn`, side by side |
| `examples/audio.lgv` | beep aligned to center crossing |
| `examples/hello.lgv` | 2-minute greeting + loader + outro (alpha fades) |
| `examples/bounce.lgv` | gravity bounces (easeIn down, easeOut up) + sloshing lines |

## How it works

```
.lgv script → Builder (Timeline) → Interpolate(t) → gg canvas → ffmpeg stdin → out.mp4
```

1. **Declare.** `vm.Run` executes the script; `scene`/`shape`/`text`/`audio` calls append to a `Builder`. Everything else (loops, prints, helpers) runs and is ignored.
2. **Interpolate.** Per entity per frame, the two surrounding keyframes blend: `f = (t-t0)/(t1-t0)` reshaped by the segment's ease curve.
3. **Rasterize.** `gg` (pure Go, no GPU/window) paints one RGBA canvas per frame. Unknown kinds and unloadable assets skip — one bad entity never kills the frame.
4. **Encode.** Raw pixels stream to `ffmpeg -f rawvideo -pix_fmt rgba -s WxH -r FPS -i -`, H.264 + AAC out. Audio inputs ride along with `-itsoffset` alignment and `apad` so short sounds never truncate the video.

## Scripting in Logos

Times are **quoted-string seconds** — Logos has no duration literals, so
`"0"` and `"1.5"` are the keys, never `0s`. Keyframes sort by time;
declare them in any order.

```js
scene(3, 30)                              // duration seconds, fps
shape("b", "circle", table{               // name, kind, keyframes
  "0": table{x: 80, y: 240, r: 40, color: "#e64c40", ease: "easeInOut"},
  "3": table{x: 560, y: 240, r: 40, color: "#40e680"}
})
text("t", table{                          // kind implied
  "0": table{content: "go", x: 320, y: 60, size: 48, color: "#ffffff", font: "/path/to.ttf"},
})
audio("assets/beep.mp3", 1.0)             // path, optional offset seconds
```

Kinds and their props (`x`/`y` in pixels, origin top-left):

- `circle`: `x y r color`
- `rect`: `x y` (top-left) `w h color`
- `line`: `x1 y1 x2 y2 width color`
- `image`: `path x y` (centered)
- `text`: `content x y size color font?`

Every vector kind also takes `alpha` (0–1, default 1) for real fades.
Images ignore it.

Missing numeric props fall back to canvas-centered defaults; missing
`path` or unloadable font skips that entity for the frame.

## CLI reference

```
kyfram render [--out f] [--fps n] [--width n] [--height n] <script.lgv>
kyfram view [--port n] <script.lgv>
```

`render` writes a video (default `out.mp4`). `view` serves a preview page:
scrub slider, play button, script errors shown inline; save the file and
refresh — no restart, no watcher.

## Project layout

```
timeline.go      Timeline / Entity / Keyframe / AudioTrack (pure data)
interpolate.go   Interpolate + easing + hex helpers
render.go        RenderFrame / RenderSequence / Encode (gg + ffmpeg pipe)
bindings.go      Builder: scene/shape/text/audio builtins
cmd/kyfram/      render + view commands, sandboxed VM init
cmd/example/     throwaway manual probe (sequence/mp4 dumping)
examples/        one .lgv per feature + assets/
logos_reference.md  pinned Logos v0.4.9 notes (local only, gitignored)
```

## Limitations

- 2D screen space only. No camera, no 3D, no streaming, no GUI editor.
- Fixed prop vocabulary per kind; no rotation, scale, or groups yet.
- `ffmpeg` and the default font are system dependencies, not embedded.
- Images decode from disk per frame (marked `ponytail:` in `render.go`); cache if it measures slow.
- Audio has offsets but no looping — keep assets covering the duration.
- Colors are hex (`#rrggbb`/`#rgb`); a name→hex mapper is planned so scripts read friendlier.
- Scripts are "declarations only" by convention — the sandbox can't block `for`/`if`, the builder just ignores them.
