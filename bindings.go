package kyfram

import (
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
	curScene int // index into tl.Scenes, -1 while declarations are global
}

// Bind registers the declaration builtins on vm. Call before Run, then
// Build the Timeline after Run returns.
func (b *Builder) Bind(vm *interpreter.Interpreter) {
	b.curScene = -1
	vm.Register("scene", func(args ...interpreter.Object) interpreter.Object {
		if len(args) < 2 || len(args) > 4 {
			return &interpreter.Error{Message: fmt.Sprintf("scene() takes 2-4 arguments, got %d", len(args))}
		}
		dur, err := numArg(args[0], "duration")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		fps, err := numArg(args[1], "fps")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		tr := "cut"
		trDur := 0.0
		if len(args) >= 3 {
			tr, err = strArg(args[2], "transition")
			if err != nil {
				return &interpreter.Error{Message: err.Error()}
			}
			if !contains(transitionModes, tr) {
				return &interpreter.Error{Message: fmt.Sprintf("scene: unknown transition %q (want one of %v)%s", tr, transitionModes, suggest(transitionModes, tr))}
			}
		}
		if len(args) == 4 {
			trDur, err = numArg(args[3], "transition duration")
			if err != nil {
				return &interpreter.Error{Message: err.Error()}
			}
		}
		b.tl.FPS = int(fps)
		b.tl.Scenes = append(b.tl.Scenes, Scene{
			Name:          fmt.Sprintf("scene%d", len(b.tl.Scenes)+1),
			Duration:      dur,
			Transition:    tr,
			TransitionDur: trDur,
		})
		b.curScene = len(b.tl.Scenes) - 1
		b.sceneSet = true
		return interpreter.NULL
	})
	vm.Register("shape", func(args ...interpreter.Object) interpreter.Object {
		if len(args) < 3 || len(args) > 4 {
			return &interpreter.Error{Message: fmt.Sprintf("shape() takes 3 arguments plus optional group, got %d", len(args))}
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
			return &interpreter.Error{Message: fmt.Sprintf("shape %q: %v", name, err)}
		}
		group := ""
		if len(args) == 4 {
			group, err = strArg(args[3], "group")
			if err != nil {
				return &interpreter.Error{Message: fmt.Sprintf("shape %q: %v", name, err)}
			}
		}
		b.addEntity(name, kind, kfs, group)
		return interpreter.NULL
	})
	vm.Register("text", func(args ...interpreter.Object) interpreter.Object {
		if len(args) < 2 || len(args) > 3 {
			return &interpreter.Error{Message: fmt.Sprintf("text() takes 2 arguments plus optional group, got %d", len(args))}
		}
		name, err := strArg(args[0], "name")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		kfs, err := keyframesArg(args[1])
		if err != nil {
			return &interpreter.Error{Message: fmt.Sprintf("text %q: %v", name, err)}
		}
		group := ""
		if len(args) == 3 {
			group, err = strArg(args[2], "group")
			if err != nil {
				return &interpreter.Error{Message: fmt.Sprintf("text %q: %v", name, err)}
			}
		}
		b.addEntity(name, "text", kfs, group)
		return interpreter.NULL
	})
	vm.Register("group", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 2 {
			return &interpreter.Error{Message: fmt.Sprintf("group() takes 2 arguments, got %d", len(args))}
		}
		name, err := strArg(args[0], "name")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		kfs, err := keyframesArg(args[1])
		if err != nil {
			return &interpreter.Error{Message: fmt.Sprintf("group %q: %v", name, err)}
		}
		b.addGroup(name, kfs)
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
			return &interpreter.Error{Message: fmt.Sprintf("camera: %v", err)}
		}
		b.addCamera(kfs)
		return interpreter.NULL
	})
	vm.Register("transparent", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 1 {
			return &interpreter.Error{Message: fmt.Sprintf("transparent() takes 1 argument, got %d", len(args))}
		}
		bval, err := boolArg(args[0], "transparent")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		b.tl.Transparent = bval
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

	// sin/cos can't be expressed in-logos (no trig in std/math); bind
	// them here so keyframe tables can be computed with loops.
	vm.Register("pi", func(args ...interpreter.Object) interpreter.Object {
		return &interpreter.Float{Value: math.Pi}
	})
	vm.Register("sin", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 1 {
			return &interpreter.Error{Message: "sin(): exactly 1 argument required"}
		}
		f, err := numArg(args[0], "angle")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		return &interpreter.Float{Value: math.Sin(f)}
	})
	vm.Register("cos", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 1 {
			return &interpreter.Error{Message: "cos(): exactly 1 argument required"}
		}
		f, err := numArg(args[0], "angle")
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		return &interpreter.Float{Value: math.Cos(f)}
	})

	// stamp(base, overrides) returns a new keyframe table where
	// every keyframe in base gets the overrides merged on top. Useful for
	// stamping out N copies of an animation with one field changed.
	vm.Register("stamp", func(args ...interpreter.Object) interpreter.Object {
		if len(args) != 2 {
			return &interpreter.Error{Message: "stamp(base, overrides): exactly 2 arguments required"}
		}
		base, ok := args[0].(*interpreter.Table)
		if !ok {
			return &interpreter.Error{Message: "stamp: first argument must be a table"}
		}
		over, ok := args[1].(*interpreter.Table)
		if !ok {
			return &interpreter.Error{Message: "stamp: second argument must be a table"}
		}
		result := make(map[string]interpreter.Object, len(base.Pairs))
		for timeKey, kfObj := range base.Pairs {
			kf, ok := kfObj.(*interpreter.Table)
			if !ok {
				continue
			}
			merged := make(map[string]interpreter.Object, len(kf.Pairs)+len(over.Pairs))
			for k, v := range kf.Pairs {
				merged[k] = v
			}
			for k, v := range over.Pairs {
				merged[k] = v
			}
			result[timeKey] = &interpreter.Table{Pairs: merged}
		}
		return &interpreter.Table{Pairs: result}
	})
}

// scOffset returns the absolute time where the open scene starts
// (sum of completed scene durations). Declarations before any scene()
// (curScene == -1) get no offset and land on the global lists.
func (b *Builder) scOffset() float64 {
	var s float64
	for i := 0; i < b.curScene && i < len(b.tl.Scenes); i++ {
		s += b.tl.Scenes[i].Duration
	}
	return s
}

func (b *Builder) addEntity(name, kind string, kfs []Keyframe, group string) {
	if off := b.scOffset(); off > 0 {
		for i := range kfs {
			kfs[i].Time += off
		}
	}
	if b.curScene >= 0 {
		sc := &b.tl.Scenes[b.curScene]
		sc.Entities = append(sc.Entities, Entity{Name: name, Kind: kind, Keyframes: kfs, Group: group})
		return
	}
	b.tl.Entities = append(b.tl.Entities, Entity{Name: name, Kind: kind, Keyframes: kfs, Group: group})
}

func (b *Builder) addGroup(name string, kfs []Keyframe) {
	if off := b.scOffset(); off > 0 {
		for i := range kfs {
			kfs[i].Time += off
		}
	}
	if b.curScene >= 0 {
		sc := &b.tl.Scenes[b.curScene]
		sc.Groups = append(sc.Groups, Group{Name: name, Keyframes: kfs})
		return
	}
	b.tl.Groups = append(b.tl.Groups, Group{Name: name, Keyframes: kfs})
}

func (b *Builder) addCamera(kfs []Keyframe) {
	if off := b.scOffset(); off > 0 {
		for i := range kfs {
			kfs[i].Time += off
		}
	}
	if b.curScene >= 0 {
		sc := &b.tl.Scenes[b.curScene]
		sc.Camera = kfs
		return
	}
	b.tl.Camera = kfs
}

// Build returns the declared Timeline. A script with entities but no
// scene() has no header to render under, so that's an error. Scenes
// total their durations into Duration. Entities then pass fail-fast
// validation (kind/prop/group checks).
func (b *Builder) Build() (Timeline, error) {
	if !b.sceneSet {
		return Timeline{}, fmt.Errorf("no scene() declaration")
	}
	if len(b.tl.Scenes) > 0 {
		var d float64
		for _, s := range b.tl.Scenes {
			d += s.Duration
		}
		b.tl.Duration = d
	}
	if err := b.validate(); err != nil {
		return Timeline{}, err
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
