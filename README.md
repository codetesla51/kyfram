# kyfram

[![pipeline](https://gitlab.com/codetesla51/kyfram/badges/master/pipeline.svg)](https://gitlab.com/codetesla51/kyfram/-/pipelines)
[![Go](https://img.shields.io/badge/go-1.25-blue)](https://go.dev)
[![Release](https://img.shields.io/gitlab/v/release/codetesla51/kyfram)](https://gitlab.com/codetesla51/kyfram/-/releases)

<p align="center">
  <img src="logo-bg.png" width="120" alt="kyfram logo — two states connected by an eased trail with frame ticks">
</p>
<p align="center"><sub>Logo made with kyfram</sub></p>

> Keyframe-driven rendering engine — declare what shapes look like at specific times, Go renders to `mp4`/`png` via `gg` + `ffmpeg`.

**Docs → [codetesla51.gitlab.io/kyfram](https://codetesla51.gitlab.io/kyfram)** — install, quickstart, and reference. This README is the quick pitch.

## What is kyfram?

`kyfram` is a small, opinionated engine for motion graphics. You write a `.lgv` script in [Logos](https://gitlab.com/codetesla51/logos) declaring states-at-times — `scene()`, `shape()`, `text()`, `group()`, `camera()`, `audio()` — and Go owns the tweening: `Timeline → Interpolate(t) → gg canvas → ffmpeg`.

Think CSS `@keyframes` for video: retiming is editing a number, not re-directing callbacks.

## Pronunciation

**kyfram** — *KAI-fram* /kaɪ.fræm/, like **keyframe** without the *e*.

## Features

- **Declarative keyframes** — times are quoted strings (`"0"`, `"1.5"`), engine lerps numbers/colors/arrays and snaps text.
- **Easing per segment** — `linear`, `easeIn/Out/InOut`, `cubicIn/Out/InOut`, `sine`.
- **Rich primitives** — `circle`, `rect`, `roundRect`, `polygon`, `star`, `ellipse`, `line`, `arc`, `wave`, `path`, `image`, `text`.
- **Groups** — `group()` with local `x/y`, keyframed `x y scale rotate alpha layer`.
- **Camera** — keyframed pan/zoom for the whole scene.
- **Style** — gradients (`gradFrom`/`gradTo`), blend modes, alpha/rotate/scale/layer per entity.
- **Audio + TTS** — `audio()` mux and `tts()` (Edge neural → `espeak-ng` → beeps) with word timings for synced text.
- **Preview + templates** — `kyfram view` hot-reload, `render --var k=v` and `text_file()` for batch renders.

See docs for the full scripting reference.

## Install

```sh
curl -fsSL https://codetesla51.gitlab.io/kyfram/install.sh | sh
```

Specific version or update:

```sh
curl -fsSL https://codetesla51.gitlab.io/kyfram/install.sh | sh -s -- v0.2.0
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
go run ./cmd/kyfram view examples/intro.lgv   # http://localhost:8080
```

Requires `ffmpeg` on `PATH` (mp4 encoding; png renders work without it).
