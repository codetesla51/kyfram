package kyfram

import (
	"fmt"
	"sort"
	"strconv"

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
		kfs = append(kfs, Keyframe{Time: t, Props: props})
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
