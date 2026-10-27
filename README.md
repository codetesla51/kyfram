# kyfram

[![pipeline](https://gitlab.com/codetesla51/kyfram/badges/master/pipeline.svg)](https://gitlab.com/codetesla51/kyfram/-/pipelines)
[![Go](https://img.shields.io/badge/go-1.25-blue)](https://go.dev)
[![Release](https://img.shields.io/gitlab/v/release/codetesla51/kyfram)](https://gitlab.com/codetesla51/kyfram/-/releases)

<p align="center">
  <img src="assets/logo-bg.png" width="120" alt="kyfram logo — two states connected by an eased trail with frame ticks">
</p>
<p align="center"><sub>Logo made with kyfram</sub></p>

> Keyframe-driven rendering engine — declare what shapes look like at specific times, Go renders to `mp4`/`png` via `gg` + `ffmpeg`.

**Docs → [kyfram-site-68056d.gitlab.io](https://kyfram-site-68056d.gitlab.io/index.html)** — install, quickstart, and reference. This README is the quick pitch.

## What is kyfram?

`kyfram` is a small, opinionated engine for motion graphics. You write a `.lgv` script in [Logos](https://gitlab.com/codetesla51/logos) declaring states-at-times — `scene()`, `shape()`, `text()`, `group()`, `camera()`, `audio()` — and Go owns the tweening: `Timeline → Interpolate(t) → gg canvas → ffmpeg`.

Think CSS `@keyframes` for video: retiming is editing a number, not re-directing callbacks.

## Pronunciation

**kyfram** — *KAI-fram* /kaɪ.fræm/, like **keyframe** without the *e*.

## Features

- **Declarative keyframes** — times are quoted strings (`"0"`, `"1.5"`), engine lerps numbers/colors/arrays and snaps text.
- **Easing per segment** — `linear`, `easeIn/Out/InOut`, `cubicIn/Out/InOut`, `sine`, `spring`, `bounceIn/Out/InOut`.
- **One-line entrances** — `fadeIn()`, `slideIn()`, `popIn()` synthesize keyframe tables; pass a start time and duration when needed.
- **Per-letter text** — `letters()` measures glyph advances and staggers each letter.
- **Stroke draw-on** — `trim` / `trimStart` reveal a `line` or `path` by length.
- **Rich primitives** — `circle`, `rect`, `roundRect`, `polygon`, `star`, `ellipse`, `line`, `arc`, `wave`, `path`, `image`, `text`.
- **Groups** — `group()` with local `x/y`, keyframed `x y scale rotate alpha layer`, and optional `parent` for nesting.
- **Camera** — keyframed pan/zoom for the whole scene.
- **Style** — gradients (`gradFrom`/`gradTo`), blend modes, alpha/rotate/scale/layer per entity.
- **Audio + TTS** — `audio()` mux and `tts()` (Edge neural → `espeak-ng` → beeps) with word timings for synced text.
- **Preview + templates** — `kyfram view` hot-reload, `render --var k=v` and `text_file()` for batch renders.

See docs for the full scripting reference.

## Install

```sh
curl -fsSL https://kyfram-site-68056d.gitlab.io/install.sh | sh
```

Specific version or update:

```sh
curl -fsSL https://kyfram-site-68056d.gitlab.io/install.sh | sh -s -- v0.2.0
# or
VERSION=v0.2.0 sh install.sh
```

Prebuilt binaries for linux/darwin (amd64/arm64) are downloaded from the
release assets; installing from source needs Go 1.25+:

```sh
# go install gitlab.com/codetesla51/kyfram/cmd/kyfram@latest
```

## Quick start

```js
// examples/intro.lgv
scene(2, 30)

shape("ball", "circle", table{
    "0": table{x: 100, y: 180, r: 40, color: "#e64c40"},
    "2": table{x: 540, y: 180, r: 40, color: "#e64c40"},
})
```

```sh
go run ./cmd/kyfram render --out out.mp4 examples/intro.lgv
go run ./cmd/kyfram view examples/intro.lgv   # http://localhost:7070
```

Requires `ffmpeg` on `PATH` (mp4 encoding; png renders work without it).

Entrance helpers are built in:

```js
shape("ball", "circle", popIn(table{x: 100, y: 180, r: 40, color: "#e64c40"}))
```

`fadeIn()`, `slideIn()`, and `popIn()` return keyframe tables. Pass a start
time and duration as optional second and third arguments. `slideIn()` also
takes a direction (`left`, `right`, `up`, `down`) and distance.

## Motion graphics recipes

`examples/motion-kit.lgv` combines all five recipes below in one 4-second
scene.

### Per-letter text

`letters(name, content, keyframes, stagger, group)` expands a line into one
`text` entity per non-space glyph. It measures advances with the first
keyframe's `font` and `size`, centers the measured line on `x`, and delays
each glyph by `stagger` seconds. Keep `font` and `size` constant across the
animation so the spacing stays stable.

```js
letters("title", "MOTION", table{
    "0": table{x: 640, y: 250, size: 96, color: "#f6f1e7", alpha: 0, scale: 0.9, ease: "spring"},
    "0.42": table{x: 640, y: 250, size: 96, color: "#f6f1e7", alpha: 1, scale: 1},
}, 0.08)
```

The optional fifth argument puts every generated glyph in a group. Spaces
advance the line but do not create an entity. Generated names are
`title-0`, `title-1`, and so on.

### Stroke draw-on

`trim` is the visible fraction of a `line` or `path` from its start;
`trimStart` moves the beginning of that window. Animate `trim` from `0` to
`1` to draw a stroke on. Paths use arc length across all subpaths. Filled
paths ignore trimming.

```js
shape("rule", "line", table{
    "0": table{x1: 300, y1: 330, x2: 980, y2: 330, width: 3, color: "#f4b942", trim: 0},
    "0.8": table{x1: 300, y1: 330, x2: 980, y2: 330, width: 3, color: "#f4b942", trim: 1},
})
```

### Nested groups

A group can name another group with `parent`. The child's local transform is
applied inside the parent, so moving or rotating the parent carries the whole
subtree. Root groups default to the canvas center; nested groups default to
their parent's origin. Each parent orders its entities and child groups by
`layer`.

```js
group("rig", table{"0": table{x: 1010, y: 490, rotate: 12}})
group("arm", table{"0": table{x: 0, y: 0, rotate: 24, parent: "rig"}})
shape("chip", "rect", table{
    "0": table{x: -46, y: -46, w: 92, h: 92, color: "#5fd0c4"}
}, "arm")
```

Parents can be declared before or after children. Build rejects missing
parents and cycles.

### Rotation pivot

`rotate` pivots around the shape's natural center:

- `rect` and `roundRect`: center of the rectangle, at `(x + w/2, y + h/2)`.
  `x` and `y` remain the top-left corner.
- `circle`, `star`, `ellipse`, and each `text` glyph: the `x/y` point.
- `line`: midpoint of the two endpoints.
- groups: the group's local origin.

### Easing

`spring` overshoots and settles; `bounce`, `bounceOut` land with four
decreasing bounces; `bounceIn` and `bounceInOut` use the same curve at the
other end or both ends. All curves start at `0` and finish exactly at `1`.
