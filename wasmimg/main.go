//go:build js && wasm

// Browser entry point for kyfram stills. Exposes the real script
// pipeline (Builder + interpolator + gg renderer) to JavaScript:
//
//	GOOS=js GOARCH=wasm go build -o kyfram.wasm ./wasmimg
//	globalThis.kyframRender(source, tSeconds) ->
//	  {ok, pngBase64?, w, h, t, error?}
//
// Image-only: no ffmpeg, audio, TTS, or filesystem. t < 0 renders the
// final frame (t = timeline duration).
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"syscall/js"

	kyfram "github.com/codetesla51/kyfram"
	"github.com/codetesla51/logos/interpreter"
)

func renderFunc(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return map[string]any{"ok": false, "error": "kyframRender: missing source argument"}
	}
	src := args[0].String()

	vm := interpreter.NewInterpreter(
		kyfram.StdLib(),
		interpreter.SandboxConfig{
			AllowFileIO:  false,
			AllowNetwork: false,
			AllowShell:   false,
			AllowExit:    false,
		},
	)
	var b kyfram.Builder
	b.Bind(vm)
	// text_file() reads the OS filesystem, which can hang the WASM
	// bridge. Override with a clean error (instance builtins win).
	vm.Register("text_file", func(args ...interpreter.Object) interpreter.Object {
		return &interpreter.Error{Message: "text_file() needs local files — not available in the browser"}
	})
	if err := vm.Run(src); err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}
	tl, err := b.Build()
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}
	}

	t := tl.Duration
	if len(args) > 1 {
		if want := args[1].Float(); want >= 0 {
			t = want
		}
	}
	frame := kyfram.RenderFrame(tl, t)
	var buf bytes.Buffer
	if err := png.Encode(&buf, frame.Image()); err != nil {
		return map[string]any{"ok": false, "error": fmt.Sprintf("png encode: %s", err)}
	}
	bounds := frame.Image().Bounds()
	return map[string]any{
		"ok":        true,
		"pngBase64": base64.StdEncoding.EncodeToString(buf.Bytes()),
		"w":         bounds.Dx(),
		"h":         bounds.Dy(),
		"t":         t,
	}
}

func main() {
	js.Global().Set("kyframRender", js.FuncOf(renderFunc))
	select {}
}
