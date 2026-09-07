package kyfram

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/codetesla51/logos/interpreter"
)

// Builder collects scene()/shape()/text() declarations out of a script.
// The script may compute and branch freely; only these calls populate
// the Timeline, everything else is ignored.
type Builder struct {
	tl       Timeline
	sceneSet bool
}

// Bind registers the declaration builtins on vm. Call before Run, then
// Build the Timeline after Run returns.
func (b *Builder) Bind(vm *interpreter.Interpreter) {
	vm.Register("scene", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 2 {
			return &interpreter.Error{Message: fmt.Sprintf("scene() takes 2 arguments, got %d", len(args))}
		}
		dur, err := numArg(args[0], "duration")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		fps, err := numArg(args[1], "fps")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		b.tl.Duration = dur
		b.tl.FPS = int(fps)
		b.sceneSet = true
		return interpreter.NULL
	})
	vm.Register("shape", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 3 {
			return &interpreter.Error{Message: fmt.Sprintf("shape() takes 3 arguments, got %d", len(args))}
		}
		name, err := strArg(args[0], "name")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		kind, err := strArg(args[1], "kind")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		kfs, err := keyframesArg(args[2])
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		b.tl.Entities = append(b.tl.Entities, Entity{Name: name, Kind: kind, Keyframes: kfs})
		return interpreter.NULL
	})
	vm.Register("text", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 2 {
			return &interpreter.Error{Message: fmt.Sprintf("text() takes 2 arguments, got %d", len(args))}
		}
		name, err := strArg(args[0], "name")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		kfs, err := keyframesArg(args[1])
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		b.tl.Entities = append(b.tl.Entities, Entity{Name: name, Kind: "text", Keyframes: kfs})
		return interpreter.NULL
	})
	vm.Register("audio", func(args ...interpreter.Object) interpreter.Object {
		if len(args) < 1 || len(args) > 3 {
			return &interpreter.Error{Message: fmt.Sprintf("audio() takes a path, optional offset, optional loop, got %d arguments", len(args))}
		}
		path, err := strArg(args[0], "path")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		track := AudioTrack{Path: path}
		if len(args) >= 2 {
			track.Offset, err = numArg(args[1], "offset")
			if err != nil {
				return &interpreter.Error{Message: err.Error()}
			}
			if track.Offset < 0 {
				return &interpreter.Error{Message: "offset must not be negative"}
			}
		}
		if len(args) == 3 {
			track.Loop, err = boolArg(args[2], "loop")
			if err != nil {
				return &interpreter.Error{Message: err.Error()}
			}
		}
		b.tl.Audio = append(b.tl.Audio, track)
		return interpreter.NULL
	})
	vm.Register("camera", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 1 {
			return &interpreter.Error{Message: fmt.Sprintf("camera() takes 1 keyframe table, got %d", len(args))}
		}
		kfs, err := keyframesArg(args[0])
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		b.tl.Camera = kfs
		return interpreter.NULL
	})
	vm.Register("text_file", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 1 {
			return &interpreter.Error{Message: fmt.Sprintf("text_file() takes 1 argument, got %d", len(args))}
		}
		path, err := strArg(args[0], "path")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		// Capped so a stray huge file can't flood the script heap.
		f, err := os.Open(path)
		if err != nil {
			return &interpreter.Error{Message: fmt.Sprintf("text_file: %v", err)}
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 1<<20))
		if err != nil {
			return &interpreter.Error{Message: fmt.Sprintf("text_file: %v", err)}
		}
		return &interpreter.String{Value: string(data)}
	})
	vm.Register("tts", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 2 {
			return &interpreter.Error{Message: fmt.Sprintf("tts() takes text and outPath, got %d", len(args))}
		}
		text, err := strArg(args[0], "text")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		out, err := strArg(args[1], "outPath")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		if strings.TrimSpace(text) == "" {
			return &interpreter.Error{Message: "tts: text must not be empty"}
		}
		res, err := synthTTS(text, out)
		if err != nil {
			return &interpreter.Error{Message: fmt.Sprintf("tts: %v", err)}
		}
		return res
	})
}

// Build returns the declared Timeline. A script with entities but no
// scene() has no header to render under, so that's an error.
func (b *Builder) Build() (Timeline, error) {
	if !b.sceneSet {
		return Timeline{}, fmt.Errorf("no scene() declaration")
	}
	return b.tl, nil
}

// keyframesArg reads table{"0": table{...}} into time-sorted Keyframes.
// Times are quoted strings because Logos has no duration literals.
func keyframesArg(obj interpreter.Object) ([]Keyframe, error) {
	raw, ok := interpreter.ObjectToGo(obj).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("keyframes must be a table")
	}
	kfs := make([]Keyframe, 0, len(raw))
	for ks, v := range raw {
		t, err := strconv.ParseFloat(ks, 64)
		if err != nil {
			return nil, fmt.Errorf("keyframe time %q: not a number", ks)
		}
		props, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("keyframe %q: value must be a table", ks)
		}
		// ease is segment metadata, not a drawable value: lift it out so
		// the renderer never sees it and blending never touches it.
		ease := ""
		if e, ok := props["ease"]; ok {
			delete(props, "ease")
			ease, ok = e.(string)
			if !ok {
				return nil, fmt.Errorf("keyframe %q: ease must be a string", ks)
			}
			if !validEase(ease) {
				return nil, fmt.Errorf("keyframe %q: unknown ease %q (want one of %v)", ks, ease, easeNames)
			}
		}
		kfs = append(kfs, Keyframe{Time: t, Props: props, Ease: ease})
	}
	sort.Slice(kfs, func(i, j int) bool { return kfs[i].Time < kfs[j].Time })
	return kfs, nil
}

// strArg unwraps a Logos string argument for name/kind positions.
func strArg(obj interpreter.Object, what string) (string, error) {
	s, ok := interpreter.ObjectToGo(obj).(string)
	if !ok {
		return "", fmt.Errorf("%s must be a string", what)
	}
	return s, nil
}

// numArg unwraps a Logos number (int or float literal) for scene header.
func numArg(obj interpreter.Object, what string) (float64, error) {
	f, ok := toFloat(interpreter.ObjectToGo(obj))
	if !ok {
		return 0, fmt.Errorf("%s must be a number", what)
	}
	return f, nil
}

// boolArg unwraps a Logos boolean for flag positions like loop.
func boolArg(obj interpreter.Object, what string) (bool, error) {
	b, ok := interpreter.ObjectToGo(obj).(bool)
	if !ok {
		return false, fmt.Errorf("%s must be a boolean", what)
	}
	return b, nil
}

type ttsSpan struct {
	word       string
	start, end float64
}

// synthTTS is a fully-local placeholder voice: it synthesizes a wav
// with one beep per word and returns word-level timings. No network,
// no model download, just ffmpeg-already-required style wav + true
// timings because we *are* the synthesizer. Swap the wav generator for
// Piper/Edge later; the API (file + timings) stays identical.
func synthTTS(text, out string) (interpreter.Object, error) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil, fmt.Errorf("no words")
	}
	spans := make([]ttsSpan, len(words))
	t := 0.0
	for i, w := range words {
		// 0.06s per rune + 0.22s base, so "hey" < "uthman", still natural
		dur := 0.22 + 0.06*float64(len([]rune(w)))
		if dur < 0.28 {
			dur = 0.28
		}
		if dur > 0.9 {
			dur = 0.9
		}
		spans[i] = ttsSpan{w, t, t + dur - 0.06}
		t += dur
	}
	total := t - 0.06
	if err := writeBeepWav(out, spans, total); err != nil {
		return nil, err
	}
	// Build Logos return: table{audio: path, duration: total, words: [table{word, start, end}]}
	wArr := make([]interpreter.Object, len(spans))
	for i, s := range spans {
		wArr[i] = &interpreter.Table{Pairs: map[string]interpreter.Object{
			"STRING:word":  &interpreter.String{Value: s.word},
			"STRING:start": &interpreter.Float{Value: s.start},
			"STRING:end":   &interpreter.Float{Value: s.end},
		}}
	}
	return &interpreter.Table{Pairs: map[string]interpreter.Object{
		"STRING:audio":    &interpreter.String{Value: out},
		"STRING:duration": &interpreter.Float{Value: total},
		"STRING:words":    &interpreter.Array{Elements: wArr},
	}}, nil
}

// writeBeepWav writes a mono 16-bit 44.1kHz wav: one sine beep per word
// (frequency steps per word so you can *hear* the word boundaries) with
// short silence gaps. Fully local, no deps beyond the Go stdlib.
func writeBeepWav(path string, spans []ttsSpan, total float64) error {
	const rate = 44100
	n := int(total * rate)
	if n <= 0 {
		n = 1
	}
	samples := make([]int16, n)
	for i, s := range spans {
		si := int(s.start * rate)
		ei := int(s.end * rate)
		freq := 180.0 + float64(i%7)*18
		for j := si; j < ei && j < n; j++ {
			// gentle envelope so clicks don't distort the music mix
			pos := float64(j-si) / float64(ei-si)
			env := math.Sin(math.Pi * pos)
			if env < 0 {
				env = 0
			}
			samples[j] = int16(9000 * env * math.Sin(2*math.Pi*freq*float64(j)/rate))
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// RIFF header
	dataSize := len(samples) * 2
	if err := binary.Write(f, binary.LittleEndian, []byte("RIFF")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(36+dataSize)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, []byte("WAVE")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, []byte("fmt ")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(1)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(rate)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(rate*2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(2)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint16(16)); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, []byte("data")); err != nil {
		return err
	}
	if err := binary.Write(f, binary.LittleEndian, uint32(dataSize)); err != nil {
		return err
	}
	return binary.Write(f, binary.LittleEndian, samples)
}
