# kyfram

> Keyframe-driven rendering engine (video + still frames) — the video is written in Logos and Go is only the runtime underneath.

`kyfram` is a small, opinionated engine for building motion-graphics clips and stills. You declare what shapes look like at specific times in a `.lgv` script — Go interpolates between keyframes, rasterizes each frame in memory with `gg` (pure Go, no GPU/window), and pipes raw pixels to `ffmpeg` for video or writes a single `png` for stills. Same lineage as `logos2d` (sibling repo, live 2D games); this one is batch rendering instead of a live window.

The mental model is CSS `@keyframes` applied to video files. Remotion, Motion Canvas, and Manim are imperative (sequences of commands). Here you declare states-at-times and the engine owns 100% of the sequencing and tweening — retiming is editing a number, not re-directing.

## What it does

- Turns a `.lgv` Logos script into an `mp4` (or a single `png` frame) via a typed `Timeline` → per-frame `Interpolate(t)` → `gg` canvas → `ffmpeg` pipe.
- Live preview server (`view`) for scrub/play/hot-reload without encoding.
- Declarative groups, rich primitives, easing, gradients, blend modes, camera, audio mux, and neural TTS — all keyframeable where it matters.

## Why it exists

Imperative video code interleaves *what* to draw with *when* to draw it. That makes retiming, reordering, and batch templating expensive: you chase timers and callbacks. `kyfram` collapses that to data — `Timeline{Duration, FPS, Width, Height, Font, Entities, Groups, Audio, Camera}` — so sequencing is a pure function of time. Scripts stay short via Logos helpers (`use`, `fn`, `for`), the engine stays boring, and one `.lgv` renders many videos via `--var`.

## Features

- **Declarative keyframes** — `scene()` / `shape()` / `text()` / `group()` / `audio()` / `camera()` / `tts()` builtins; scripts compute freely, the builder keeps only declarations.
- **Interpolator** — numbers lerp, hex/named colors lerp per-channel, numeric arrays (point lists) lerp elementwise, strings/discrete values snap to the earlier key; out-of-range holds the edge, never extrapolates; empty input returns empty.
- **Easing** — `linear | easeIn | easeOut | easeInOut | cubicIn | cubicOut | cubicInOut | sine` per segment, declared on the earlier key as `ease: "easeIn"`.
- **Groups** — `group(name, keyframes)` containers with keyframed `x y scale rotate alpha layer`; children keep local `x/y` and animate independently; group transform is `translate(gx,gy) → rotate → scale` applied to all children (child `x/y` is local to group origin); children sort by `layer` within group and groups interleave with root by `layer`.
- **Primitives** — `circle`, `rect`, `roundRect`, `polygon`, `star`, `ellipse`, `line`, `arc`, `wave`, `path`, `image`, `text`; every vector kind takes `rotate` (degrees), `scale`, `layer`, `alpha`.
- **Quality controls** — `--crf 0-51` (lower = better, 18 high, 23 default, 28 small) + `--preset ultrafast..veryslow` (medium default) exposed to `EncodeWithOpts`; single-frame `png` via `--format png` (no `ffmpeg` needed).
- **Zero-disk render path** — raw RGBA bytes piped to `ffmpeg -f rawvideo -pix_fmt rgba` on stdin; PNG sequences exist only as debug path (`RenderSequence`).
- **Audio mux** — `audio(path, offset?, loop?)` tracks mixed via `-itsoffset` + `apad`/`amix`; short sounds never truncate the video; `loop` repeats to last frame.
- **Neural TTS** — `tts(text, outPath, opts?)` tries Edge neural first (exact `WordBoundary` timings, `voice`/`rate`/`pitch`/`volume`), falls back to `espeak-ng` (local), then beeps; returns `table{audio, duration, words:[{word,start,end}]}` for word-synced animation; `tts_voices()` lists Edge voices.
- **Preview server** — `kyfram view` serves scrub slider, play button, hot reload; frames are `PNG` (not `mp4`) so saves are instant; cached by file `mtime` (first frame ~2s with Edge, next scrubs <100ms).
- **Sandboxed scripts** — `interpreter.SandboxConfig{AllowFileIO:false, AllowNetwork:false, AllowShell:false, AllowExit:false}`; Logos `fileRead` stays blocked; `text_file(path)` and `tts` are the narrow allowed builtins.
- **Multi-file scripts** — `use "lib"` inlines `lib.lgv` beside the script (Go-side loader, Logos itself never forks); `std/` resolves via vendored `third_party/` + `StdLib()` FS.
- **Templates** — `render --var key=value` (repeatable) feeds script vars via `vm.SetVar`; `text_file(path)` reads long copy from disk (1 MB cap, build-time frozen).
- **Camera** — keyframed `camera(table{"0": table{x,y,zoom}})` pans/zooms the whole scene via `frameCamera` push/pop.
- **Named colors** — 24 names (`red gold slate mint coral sky lavender ...`) resolve to hex and lerp exactly like hex; case-insensitive.
- **Blend modes** — `blend: "multiply" | "screen" | "add"` per entity (offscreen layer + `composite`).
- **Gradients** — `gradFrom/gradTo/gradAngle` linear fills on `circle/rect/polygon/roundRect/ellipse` (strokes/images stay flat).
- **Text on path** — `points` on a `text` lays glyphs along the polyline, each rotated to its segment.
- **Image cache** — assets decoded once per process, not per frame.

## Requirements

- Go 1.27+
- `ffmpeg` + `ffprobe` on `PATH` (checked: `n9.0.1`; used for `Encode` and TTS duration probing)
- A TrueType font at `/usr/share/fonts/noto/NotoSans-Regular.ttf` or override via `--font` / per-text `font:` / `Timeline.Font` (fonts must be `.ttf` — `.otf`/CFF fails under `freetype` and the text is skipped)
- Optional: `edge-tts` (`pip install edge-tts`) + `python3` for neural TTS; `espeak-ng` for local fallback (installed via `pacman`); neither is required — beeps work offline with zero deps

## Installation

```sh
git clone https://github.com/codetesla51/kyfram && cd kyfram

# Go deps
go mod download

# verify toolchain
make build   # go build ./...
make test    # go test ./... -v
make vet     # go vet ./...

# optional: Logos formatter (for .lgv)
go install github.com/codetesla51/logos@v0.4.10
logos fmt examples/*.lgv examples/helpers/*.lgv
# or without install
go run github.com/codetesla51/logos@v0.4.10 fmt examples/*.lgv

# lint (requires golangci-lint)
make lint
```

Binary:

```sh
go build -o kyfram ./cmd/kyfram
./kyfram render --help
./kyfram view --help
```

## Quick start

```sh
go run ./cmd/kyfram render examples/intro.lgv                # → out.mp4 (default)
go run ./cmd/kyfram render --out v.mp4 --crf 18 --preset slow examples/intro.lgv
go run ./cmd/kyfram render --format png --out frame.png examples/polygon.lgv
go run ./cmd/kyfram view examples/intro_voice.lgv            # → http://localhost:8080
make render   # intro_voice.lgv → out.mp4
make view     # intro_voice.lgv preview
```

Minimal script (`examples/intro.lgv`):

```js
scene(2, 30)

shape("ball", "circle", table{
    "0": table{x: 100, y: 160, r: 80, color: "#e64c40"},
    "2": table{x: 320, y: 160, r: 80, color: "#e64c40"},
})

text("caption", table{
    "0": table{content: "hello kyfram", x: 320, y: 300, size: 28, color: "#ffffff"},
    "2": table{content: "hello kyfram", x: 320, y: 300, size: 28, color: "#ffffff"},
})
```

Word-synced voice (`examples/intro_voice.lgv`):

```js
let v = tts("hey my name is uthman and this is kyfram", "/tmp/intro_voice.mp3", table{
    voice: "en-NG-EzinneNeural",
    rate: "+5%",
    pitch: "+0Hz",
    volume: "+0%",
})

scene(v.duration + 1, 30)
audio(v.audio)

shape("bg", "rect", table{
    "0": table{x: 0, y: 0, w: 640, h: 360, color: "#0a0a18"},
    str(v.duration + 1): table{x: 0, y: 0, w: 640, h: 360, color: "#0a0a18"},
})

for i, w in v.words {
    text("w" + str(i), table{
        str(w.start): table{content: w.word, x: 320, y: 180, size: 64, color: "white", alpha: 0, scale: 0.85},
        str(w.start + 0.08): table{content: w.word, x: 320, y: 180, size: 64, color: "white", alpha: 1, scale: 1},
        str(w.end): table{content: w.word, x: 320, y: 180, size: 64, color: "white", alpha: 1, scale: 1},
        str(w.end + 0.12): table{content: w.word, x: 320, y: 180, size: 64, color: "white", alpha: 0, scale: 1.08},
    })
}
```

Group (`examples/groups.lgv`):

```js
scene(3, 30)

group("g", table{
    "0": table{x: 320, y: 180, scale: 1, rotate: 0},
    "3": table{x: 380, y: 180, scale: 1.4, rotate: 25},
})

shape("c", "circle", table{
    "0": table{x: 0, y: 0, r: 28, color: "coral"},
    "3": table{x: 0, y: 0, r: 28, color: "gold"},
}, "g")

text("t", table{
    "0": table{content: "grouped", x: 0, y: 50, size: 18, color: "white"},
}, "g")

shape("solo", "rect", table{
    "0": table{x: 40, y: 280, w: 80, h: 30, color: "mint"},
    "3": table{x: 520, y: 280, w: 80, h: 30, color: "mint"},
})
```

## CLI reference

```
kyfram render [--out f] [--fps n] [--width n] [--height n] [--font f]
              [--crf n] [--preset s] [--format mp4|png] [--var k=v ...] <script.lgv>
kyfram view [--port n] <script.lgv>
```

| flag | default | meaning |
|------|---------|---------|
| `--out` | `out.mp4` | output path (`mp4` or `png` depending on `--format`) |
| `--fps` | scene fps | override timeline FPS |
| `--width` / `--height` | `640x360` | override canvas size (`Timeline.Width/Height`) |
| `--font` | `examples/assets/fonts/ArchivoBlack-Regular.ttf` | fallback typeface; precedence `text font:` prop > `--font`/`Timeline.Font` > built-in |
| `--crf` | `23` | x264 CRF 0–51, lower is better (18 visually lossless, 28 small) |
| `--preset` | `medium` | x264 preset `ultrafast superfast veryfast faster fast medium slow slower veryslow` |
| `--format` | `mp4` | `mp4` (via `ffmpeg`) or `png` (single frame at `t=0`, no `ffmpeg`) |
| `--var k=v` | — | repeatable template vars (`vm.SetVar` before `Run`) |

One script, many videos: leave names/copy as bare variables and feed per render — `kyfram render --var title="Batch Alice" examples/vars.lgv`. Long copy via `text_file(path)`.

## Scripting in Logos

Times are **quoted-string seconds** — Logos has no duration literals, so `"0"` and `"1.5"` are keys, never `0s`. Keyframes sort by time; declare in any order.

```js
scene(3, 30)                              // duration, fps
shape("b", "circle", table{               // name, kind, keyframes, optional group
    "0": table{x: 80, y: 240, r: 40, color: "#e64c40", ease: "easeInOut"},
    "3": table{x: 560, y: 240, r: 40, color: "#40e680"},
})
text("t", table{                          // kind implied
    "0": table{content: "go", x: 320, y: 60, size: 48, color: "#ffffff", font: "/path/to.ttf"},
})
group("g", table{"0": table{x: 320, y: 180, scale: 1, rotate: 0}})
shape("c", "circle", table{"0": table{x: 0, y: 0, r: 20, color: "red"}}, "g")
audio("/path/to.mp3", 1.0)                // path, offset, optional loop bool
audio(v.audio)                            // tts audio (mp3 from Edge)
camera(table{"0": table{x: 320, y: 180, zoom: 1}, "2": table{x: 400, y: 180, zoom: 1.2}})

let v = tts("hello world", "/tmp/out.mp3")                          // default voice en-US-AriaNeural
let v = tts("hello world", "/tmp/out.mp3", "en-US-JennyNeural")     // string shorthand
let v = tts("hello world", "/tmp/out.mp3", table{voice: "en-NG-EzinneNeural", rate: "+10%", pitch: "+0Hz", volume: "+0%"})
let voices = tts_voices()                 // list Edge voices
let copy = text_file("assets/copy.txt")   // 1 MB cap, frozen at build time

use "helpers/spike"                       // inlines helpers/spike.lgv beside the script
```

### Kinds and props

`x`/`y` in pixels, origin top-left. Unknown kinds and unloadable assets skip silently — one bad entity never kills the frame.

- `circle`: `x y r color`
- `rect`: `x y` (top-left) `w h color`
- `roundRect`: `x y w h radius color` — `radius` lerps.
- `polygon`: `points` flat `[x1,y1,…]` `color` `filled?` (default true) `width` — closed, same-length morphs via array lerp.
- `star`: `x y r points inner color` — `r` outer, `points` count, `inner` 0–1 inner ratio.
- `ellipse`: `x y rx ry color` (`ry` defaults to `rx`, `r` alias for `rx`).
- `line`: `x1 y1 x2 y2 width color`.
- `arc`: `x y` (center) `r start end` (degrees) `width color` — `0..360` is a ring.
- `wave`: `x y` (left baseline) `width amplitude frequency` (cycles) `phase` (degrees) `thickness color` — engine owns the sine.
- `path`: `points` flat `[x1,y1,…]` `width color filled? closed? cap join` — `cap: "round"|"butt"|"square"`, `join: "round"|"bevel"`.
- `image`: `path x y` (centered); cached decode, takes `rotate`/`scale`/`layer` but ignores `alpha`.
- `text`: `content x y size color font?` — `points` lays glyphs along the polyline.
- `group`: `group(name, table{"0": table{x,y,scale,rotate,alpha,layer}})` then `shape(..., group)` / `text(..., group)` — child `x/y` is local; group `scale/rotate/alpha` multiply children; nested groups are future (flat v1).

Every vector kind also takes `alpha` 0–1, `rotate` degrees, `scale`, `layer` (paint order, lower first), `ease` per segment. Gradients on `circle/rect/polygon/roundRect/ellipse`: `gradFrom gradTo` (hex or name) + `gradAngle`. Blend on any entity: `blend: "multiply" | "screen" | "add"`. Missing numeric props fall back to canvas-centered defaults.

## How it works

```
.lgv script → LoadScript (inline use) → vm.Run → Builder (Timeline)
          → Interpolate(t) → gg canvas → ffmpeg stdin → out.mp4
```

1. **Load.** `LoadScript` inlines local `use "name"` (`name.lgv` beside script; `std/` stays for Logos via `third_party/`).
2. **Declare.** `vm.Run` executes the script; `scene`/`shape`/`text`/`group`/`audio`/`camera`/`tts` calls append to `Builder`. Everything else (loops, helpers) runs and is ignored.
3. **Interpolate.** Per entity per frame, the two surrounding keyframes blend: `f = (t-t0)/(t1-t0)` reshaped by the earlier key's ease curve; `blend` lerps numbers/colors/arrays, snaps strings.
4. **Rasterize.** `gg` paints one RGBA canvas per frame; `frameCamera` pushes pan/zoom; groups push `translate→rotate→scale`; `drawEntity` dispatches on kind; `gradientFill` / `composite` / `spin` handle extras.
5. **Encode.** `EncodeWithOpts` streams raw `rgbaBytes` to `ffmpeg -f rawvideo -pix_fmt rgba -s WxH -r FPS -i -` with `-crf`/`-preset` and audio inputs via `-itsoffset`/`-stream_loop`/`apad`/`amix`; single `png` skips `ffmpeg` entirely.

## Examples

| script | shows |
|--------|-------|
| `examples/intro.lgv` | ball glide + caption (start here) |
| `examples/intro_voice.lgv` | Nigerian `EzinneNeural` TTS + word-synced crossfade (fixed: max 1 opaque at a time) |
| `examples/voice.lgv` | minimal TTS + `v.words` loop |
| `examples/showcase.lgv` | 3-key arc, color washes, fading beam |
| `examples/rect.lgv` | sliding, recoloring rect |
| `examples/line.lgv` | sweeping line |
| `examples/image.lgv` | image asset gliding (`assets/ball.png`) |
| `examples/text.lgv` | growing text + snapping content |
| `examples/ease.lgv` | linear vs `easeIn` side by side |
| `examples/audio.lgv` | beep aligned to center crossing |
| `examples/hello.lgv` | 2-minute greeting + loader + outro (alpha fades) |
| `examples/bounce.lgv` | gravity bounces (`easeIn` down, `easeOut` up) + sloshing lines |
| `examples/spinner.lgv` | ring + rotating arc hand |
| `examples/vars.lgv` | `--var` templating + `text_file` |
| `examples/wave.lgv` | morphing wave, gradient sun, blend halo, path sail, text on path |
| `examples/camera.lgv` | keyframed pan and zoom |
| `examples/groups.lgv` | group orbit/scale/rotate with children (local + world) |
| `examples/polygon.lgv` | filled morphing polygon |
| `examples/roundrect.lgv` | animating rounded rect + radius |
| `examples/star.lgv` | 5-point star, inner ratio + rotate |
| `examples/ellipse.lgv` | ellipse `rx/ry` vs circle `r` |
| `examples/dash.lgv` | 10s product cut, helpers `spikeAt`/`addJump`, `Stereo Madness` audio, camera pans |
| `examples/helpers/spike.lgv` | `spikeAt(name, arrival, color)` helper |
| `examples/helpers/cube.lgv` | `addJump(keys,t,rot)` helper |

Run any:

```sh
go run ./cmd/kyfram render examples/<name>.lgv --out /tmp/<name>.mp4
mpv /tmp/<name>.mp4
go run ./cmd/kyfram render --format png --out /tmp/<name>.png examples/<name>.lgv
mpv /tmp/<name>.png
```

## Project layout

```
timeline.go        Timeline / Entity / Group / Keyframe / AudioTrack (pure data)
interpolate.go     Interpolate + easing + hex/named-color + array blend
render.go          RenderFrame / RenderSequence / EncodeWithOpts (gg + ffmpeg pipe, groups)
bindings.go        Builder: scene/shape/text/group/audio/camera/text_file/tts/tts_voices
loader.go          LoadScript: Go-side use inliner
stdlib.go          StdLib() FS for vendored Logos stdlib
fonts.go           DisplayFont() — bundled Archivo Black
cmd/kyfram/        render + view commands, sandboxed VM init, view cache by mtime
cmd/example/       throwaway manual probe (sequence/mp4 dumping)
examples/          one .lgv per feature + assets/ + helpers/
third_party/       vendored Logos stdlib (.lgs) for use "std/..."
logos_reference.md pinned Logos v0.4.10 notes (local only, gitignored)
Makefile           build/test/vet/fmt/lint/render/view/clean
.golangci.yml      v2, govet/staticcheck/unused/misspell
.github/workflows/ ci.yml (build→vet→test→fmt check→lint), release.yml (tag v* → cross-build)
```

## Development

```sh
make build   # go build ./...
make test    # go test ./... -v
make vet     # go vet ./...
make fmt     # gofmt -w . + logos fmt examples/*.lgv helpers/*.lgv (requires logos v0.4.10)
make lint    # golangci-lint run
make render  # render intro_voice.lgv → out.mp4
make view    # preview intro_voice.lgv → :8080
make clean   # rm -f out.mp4 intro_voice.mp4 /tmp/*.mp4
```

Formatter notes: `lgs fmt` only walks `*.lgs` — for `.lgv` run `logos fmt examples/*.lgv` or `go run github.com/codetesla51/logos@v0.4.10 fmt examples/*.lgv`. Fixed in `v0.4.10` to preserve `for i, w in` index; older versions drop `i`.

## Limitations

- 2D screen space only. No 3D, streaming, or GUI editor.
- Groups are flat (one level) for `v1`; nested groups are next.
- Gradient fills on circle/rect/polygon/roundRect/ellipse; strokes/images stay flat.
- `ffmpeg` and the default font are system dependencies, not embedded. Fonts must be `.ttf` — `.otf`/CFF fails and text is skipped.
- Audio loops whole files; no per-section repeat.
- Scripts are "declarations only" by convention — sandbox can't block `for`/`if`, builder just ignores them.
- TTS `edge-tts` needs network; offline falls back to `espeak-ng` (proportional timings) then beeps.
```

