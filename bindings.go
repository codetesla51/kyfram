package kyfram

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

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
		if len(args) < 2 || len(args) > 3 {
			return &interpreter.Error{Message: fmt.Sprintf("tts() takes text, outPath, optional opts, got %d", len(args))}
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
		opts := ttsOpts{voice: "en-US-AriaNeural", rate: "+0%", pitch: "+0Hz", volume: "+0%"}
		if len(args) == 3 {
			o, err := parseTTSOpts(args[2])
			if err != nil {
				return &interpreter.Error{Message: fmt.Sprintf("tts opts: %v", err)}
			}
			// overlay non-empty fields
			if o.voice != "" {
				opts.voice = o.voice
			}
			if o.rate != "" {
				opts.rate = o.rate
			}
			if o.pitch != "" {
				opts.pitch = o.pitch
			}
			if o.volume != "" {
				opts.volume = o.volume
			}
		}
		res, err := synthTTS(text, out, opts)
		if err != nil {
			return &interpreter.Error{Message: fmt.Sprintf("tts: %v", err)}
		}
		return res
	})
	vm.Register("tts_voices", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 0 {
			return &interpreter.Error{Message: fmt.Sprintf("tts_voices() takes no arguments, got %d", len(args))}
		}
		voices, err := listEdgeVoices()
		if err != nil {
			return &interpreter.Error{Message: fmt.Sprintf("tts_voices: %v", err)}
		}
		arr := make([]interpreter.Object, len(voices))
		for i, v := range voices {
			arr[i] = &interpreter.String{Value: v}
		}
		return &interpreter.Array{Elements: arr}
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

type ttsOpts struct {
	voice  string
	rate   string
	pitch  string
	volume string
}

// parseTTSOpts reads the 3rd tts arg: string shorthand or table{voice,rate,pitch,volume}
func parseTTSOpts(obj interpreter.Object) (ttsOpts, error) {
	var o ttsOpts
	// string shorthand: tts("hi","/tmp/o.mp3","en-US-JennyNeural")
	if s, ok := interpreter.ObjectToGo(obj).(string); ok {
		if strings.TrimSpace(s) == "" {
			return o, fmt.Errorf("voice must not be empty")
		}
		o.voice = s
		return o, nil
	}
	m, ok := interpreter.ObjectToGo(obj).(map[string]interface{})
	if !ok {
		return o, fmt.Errorf("opts must be a string voice or table{voice,rate,pitch,volume}")
	}
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			return o, fmt.Errorf("opts.%s must be a string", k)
		}
		switch k {
		case "voice":
			o.voice = s
		case "rate":
			o.rate = s
		case "pitch":
			o.pitch = s
		case "volume":
			o.volume = s
		default:
			return o, fmt.Errorf("unknown opts field %q (want voice, rate, pitch, volume)", k)
		}
	}
	return o, nil
}

// edgePythonCode is the inline helper that talks to Edge via word-boundary stream.
// No helper file on disk — Go ships this string and execs python3 -c.
const edgePythonCode = `
import asyncio, argparse, sys
from edge_tts import Communicate
async def main():
    p=argparse.ArgumentParser()
    p.add_argument("--text", required=True)
    p.add_argument("--voice", default="en-US-AriaNeural")
    p.add_argument("--rate", default="+0%")
    p.add_argument("--volume", default="+0%")
    p.add_argument("--pitch", default="+0Hz")
    p.add_argument("--write-media", required=True)
    p.add_argument("--write-metadata", required=True)
    args=p.parse_args()
    comm=Communicate(args.text, voice=args.voice, rate=args.rate, volume=args.volume, pitch=args.pitch, boundary="WordBoundary")
    await comm.save(args.write_media, args.write_metadata)
asyncio.run(main())
`

// synthTTS tries Edge (network, neural, word-exact), then espeak-ng (local), then beeps.
// Edge path returns exact WordBoundary timings; espeak path estimates proportionally.
func synthTTS(text, out string, opts ttsOpts) (interpreter.Object, error) {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil, fmt.Errorf("no words")
	}
	// 1) Edge TTS — exact word timings, best quality
	if res, err := tryEdgeTTS(text, out, opts); err == nil {
		return res, nil
	}
	// 2) espeak-ng — local, no network
	if err := exec.Command("espeak-ng", "-v", "en", "-s", "150", "-w", out, text).Run(); err == nil {
		if fi, err := os.Stat(out); err == nil && fi.Size() > 44 {
			raw := make([]float64, len(words))
			sum := 0.0
			for i, w := range words {
				d := 0.22 + 0.06*float64(len([]rune(w)))
				if d < 0.28 {
					d = 0.28
				}
				if d > 0.9 {
					d = 0.9
				}
				raw[i] = d
				sum += d
			}
			total := sum - 0.06
			if d, err := wavDuration(out); err == nil && d > 0.3 {
				total = d
			}
			spans := make([]ttsSpan, len(words))
			t := 0.0
			for i, w := range words {
				d := raw[i] / sum * total
				if i == len(words)-1 {
					d = total - t
				}
				spans[i] = ttsSpan{w, t, t + d - 0.04}
				if spans[i].end < spans[i].start+0.15 {
					spans[i].end = spans[i].start + 0.15
				}
				t += d
			}
			return ttsTable(out, total, spans)
		}
	}
	// 3) Fallback: beeps
	spans := make([]ttsSpan, len(words))
	t := 0.0
	for i, w := range words {
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
	return ttsTable(out, total, spans)
}

// tryEdgeTTS calls the inline python helper with WordBoundary, parses metadata, returns ttsTable.
// On any failure (no python, no edge-tts, no network, timeout) it returns error to trigger fallback.
func tryEdgeTTS(text, out string, opts ttsOpts) (interpreter.Object, error) {
	if strings.TrimSpace(opts.voice) == "" {
		opts.voice = "en-US-AriaNeural"
	}
	if opts.rate == "" {
		opts.rate = "+0%"
	}
	if opts.pitch == "" {
		opts.pitch = "+0Hz"
	}
	if opts.volume == "" {
		opts.volume = "+0%"
	}
	// write to out directly (mp3) — Encode handles mp3 via ffmpeg, no wav conversion needed
	metaPath := out + ".edge.json"
	defer os.Remove(metaPath)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "python3", "-c", edgePythonCode,
		"--text", text,
		"--voice", opts.voice,
		"--rate", opts.rate,
		"--pitch", opts.pitch,
		"--volume", opts.volume,
		"--write-media", out,
		"--write-metadata", metaPath,
	)
	// keep stdout/stderr for debugging if it fails
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("edge-tts: %v (%s)", err, strings.TrimSpace(string(outBytes)))
	}
	fi, err := os.Stat(out)
	if err != nil || fi.Size() < 100 {
		return nil, fmt.Errorf("edge-tts: no audio produced")
	}
	spans, err := parseEdgeMetadata(metaPath)
	if err != nil {
		return nil, err
	}
	if len(spans) == 0 {
		return nil, fmt.Errorf("edge-tts: no word boundaries")
	}
	// total is ffprobe duration if available, else last word end + tail
	total := 0.0
	if d, err := probeDuration(out); err == nil && d > 0.3 {
		total = d
	} else {
		total = spans[len(spans)-1].end + 0.15
	}
	// Edge offsets are ticks (100ns): already converted in parser; ensure last span doesn't exceed total
	if spans[len(spans)-1].end > total {
		total = spans[len(spans)-1].end + 0.05
	}
	return ttsTable(out, total, spans)
}

// parseEdgeMetadata reads the json-lines file written by the helper: one WordBoundary per line.
func parseEdgeMetadata(path string) ([]ttsSpan, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var spans []ttsSpan
	dec := json.NewDecoder(f)
	for {
		var m struct {
			Type     string `json:"type"`
			Offset   int64  `json:"offset"`
			Duration int64  `json:"duration"`
			Text     string `json:"text"`
		}
		if err := dec.Decode(&m); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if m.Type != "WordBoundary" {
			continue
		}
		start := float64(m.Offset) / 1e7
		end := start + float64(m.Duration)/1e7
		// Edge sometimes returns empty text for punctuation; skip empties
		t := strings.TrimSpace(m.Text)
		if t == "" {
			continue
		}
		spans = append(spans, ttsSpan{word: t, start: start, end: end})
	}
	return spans, nil
}

// probeDuration uses ffprobe to get audio duration (works for mp3 and wav). Falls back to wavDuration.
func probeDuration(path string) (float64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", path)
	out, err := cmd.Output()
	if err == nil {
		s := strings.TrimSpace(string(out))
		if d, err := strconv.ParseFloat(s, 64); err == nil && d > 0 {
			return d, nil
		}
	}
	// fallback for wav
	return wavDuration(path)
}

// listEdgeVoices runs edge-tts --list-voices and returns short names, or a small hardcoded fallback.
func listEdgeVoices() ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "edge-tts", "--list-voices")
	out, err := cmd.Output()
	if err != nil {
		// ponytail: hardcoded fallback, Edge not installed or offline
		return []string{
			"en-US-AriaNeural", "en-US-JennyNeural", "en-US-GuyNeural",
			"en-GB-SoniaNeural", "en-GB-RyanNeural",
			"en-AU-NatashaNeural", "ja-JP-NanamiNeural",
		}, nil
	}
	var voices []string
	for _, line := range strings.Split(string(out), "\n") {
		// first column is Name
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if strings.HasSuffix(name, "Neural") {
			voices = append(voices, name)
		}
	}
	if len(voices) == 0 {
		return nil, fmt.Errorf("no voices found")
	}
	return voices, nil
}

// ttsTable builds the Logos return value shared by all paths.
func ttsTable(out string, total float64, spans []ttsSpan) (interpreter.Object, error) {
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

// wavDuration reads a wav header to get duration without ffprobe.
func wavDuration(path string) (float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	var hdr [44]byte
	if _, err := io.ReadFull(f, hdr[:]); err != nil {
		return 0, err
	}
	rate := binary.LittleEndian.Uint32(hdr[24:28])
	blockAlign := binary.LittleEndian.Uint16(hdr[32:34])
	dataSize := binary.LittleEndian.Uint32(hdr[40:44])
	if rate == 0 || blockAlign == 0 {
		return 0, fmt.Errorf("bad wav header")
	}
	return float64(dataSize) / float64(uint32(blockAlign)*rate/2*2) * 1.0, nil
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
