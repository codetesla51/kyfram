# kyfram — plan

## What this is

A declarative, keyframe-driven video engine built on the Logos scripting
language. Same lineage as logos2d (sibling repo, `../logos2d`), but batch
instead of live: a `.lgv` script declares what shapes look like at specific
times, Go interpolates between keyframes, draws each frame with `gg`
(pure Go, no GPU/window), and pipes raw frames to `ffmpeg` for encoding.

The mental model is CSS `@keyframes` applied to video files. Remotion,
Motion Canvas, and Manim are all imperative (sequence commands). Nothing
mainstream lets you declare states-at-times and have the engine own 100%
of the sequencing and tweening.

Status: scaffold only. `main.go` inits a sandboxed VM and runs one script
file. No Timeline, no renderer, no bindings yet.

## Decisions already made (and why)

- **Raw `interpreter.Interpreter`, not the `logos.VM` wrapper.** The wrapper
  only exposes Run/Call-by-name/Register. We need `Eval` plus raw
  `logos.Object` access to read keyframe tables back out of the VM. Same
  reason logos2d uses the interpreter directly (`vm.go` there).
- **`.lgv` entry extension is our own convention.** `vm.Run` takes a source
  string and never checks filenames, so any extension works. Exception:
  `use "x"` inside scripts is resolved by Logos itself and hardcodes
  `x.lgs`. Multi-file support will be a small Go-side loader (run each
  dep file through `vm.Run` in order, strip `use` lines, run main), not a
  Logos fork.
- **Keyframe syntax uses plain numbers, not `0s`/`4s`.** Logos has no
  duration literals, so `0s: {...}` does not parse. Times are float
  seconds as quoted-string keys: `table{"0": table{...}, "1": table{...}}`.
  Quoted strings read back unambiguously in Go (`STRING:0`).
- **No logic ban in scripts.** The sandbox blocks file IO, network, shell,
  and exit. It cannot block `for`/`if`. "Scripts are declarations only" is
  a documented convention; the Timeline builder simply ignores everything
  except declaration calls.
- **`ffmpeg` is an external dependency.** "Single binary" really means
  single Go binary plus a system ffmpeg. Verified present (n9.0.1).
- **Text input has two paths.** CLI flags via `vm.SetVar` (for templates,
  non-programmers) and a future `text_file()` Go builtin (Logos's own
  `fileRead` stays sandbox-blocked; our builtin is allowed because we
  register it). Flags first, `text_file` only when long copy needs it.

## What could break / known risks

- `gg` text handling (font loading, alignment, wrapping) is the most
  likely pain point. Prove it early with one text frame before building
  the full primitive vocabulary.
- ffmpeg stdin piping: pixel format, frame size, and fps must match the
  flags exactly or output is corrupt with a cryptic error. Log the exact
  command on failure.
- Color interpolation must be per-channel on parsed hex, not string lerp.
  Discrete fields (text content, font) snap to the earlier keyframe.
- Past-the-last-keyframe time holds the last value, never extrapolates.
- Audio alignment (start offsets, loop) needs `-filter_complex` care; it
  comes late on purpose, after silent video is solid.

## Out of scope (deliberately excluded)

- 3D, cameras, depth, meshes. 2D screen space only.
- Live/real-time use: no webcam, streaming, or footage compositing.
- Timeline-scrubbing GUI editor. A `view` preview command eventually,
  never a full NLE.
- Audio analysis or reactivity. Audio is muxed, not driven by.
- Groups/layers, transitions, easing beyond linear, templates. All
  deferred until a silent linear single-scene video renders end to end.
- Forking Logos. If `use` or syntax needs change, do it Go-side.

## Steps (each independently checkable)

1. Static frame proof: raw `gg`, no VM. Draw one circle plus one text
   line, `SavePNG`. Done when `out.png` looks right.
2. Timeline struct: `Timeline{Duration, FPS, Entities}`, `Entity{Name,
   Kind, Keyframes}`, `Keyframe{Time float64, Props map[string]any}`.
   Pure data, hand-built in a test. Done when a literal round-trips.
3. Interpolator: `Interpolate(kfs, t)` — lerp numbers, per-channel
   colors, snap strings. Unit tests at t=0, midpoint, end, past-end.
4. RenderFrame: `RenderFrame(tl, t)` dispatches to `gg` per Kind on a
   hand-built Timeline. Done when the t=2.0 PNG matches expectation.
5. PNG sequence: loop frames to `frame_%04d.png`. Done when flipping
   through them looks like smooth motion.
6. ffmpeg pipe: raw RGBA bytes to stdin,
   `ffmpeg -f rawvideo -pix_fmt rgba -s WxH -r FPS -i - out.mp4`. Done
   when `out.mp4` plays correctly.
7. Bindings: `scene()`/`shape()`/`text()` builtins populate Timeline
   from a real `.lgv` (quoted-string time keys). Done when
   `kyfram render examples/intro.lgv` produces a video.
8. Vocabulary: rect, line, image, font-backed text. One example script
   each.
9. Easing: named curves on keyframe segments. Done when two renders
   with different easings visibly differ.
10. CLI: `render` with `--out/--fps/--width/--height`. Works from a
    clean checkout.
11. `view`: live preview with play/scrub plus hot reload. Only after
    render is solid.
12. Audio: `audio()` builtin plus ffmpeg mux. Aligned to a visual beat.
13. Docs pass mirroring logos2d README structure. Done when a stranger
    can write a scene from docs alone.

## Repo map for the next agent

- `main.go` — VM init (`newVM`) plus file load and `vm.Run`. Grows a
  `render` command in step 7.
- `plan.md` (this file) — context and build order.
- `examples/intro.lgv` — smoke script proving `vm.Run` works.
- `../logos2d` — reference implementation of the pattern: `vm.go`
  (sandboxed VM), `bindings.go` (builtin registration and
  Logos-to-Go value conversion), `ecs.go` (`callClosure` shows how to
  `Eval` stored functions). Read `bindings.go` before writing step 7.
- Deps: `github.com/codetesla51/logos v0.4.9`,
  `github.com/fogleman/gg v1.3.0`, system `ffmpeg`.
