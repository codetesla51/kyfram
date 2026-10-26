package kyfram

import (
	"fmt"
	"strconv"

	"github.com/codetesla51/logos/interpreter"
)

// bindAnimationHelpers adds the one-line entrance shorthands. Each helper
// returns a normal keyframe table, so it composes with shape/text/group
// exactly like a handwritten animation and remains retimeable.
func bindAnimationHelpers(vm *interpreter.Interpreter) {
	vm.Register("fadeIn", func(args ...interpreter.Object) interpreter.Object {
		props, at, dur, err := helperTiming("fadeIn", args, 0.35, 3)
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		target, err := tableNumber(props, "alpha", 1)
		if err != nil {
			return &interpreter.Error{Message: "fadeIn: " + err.Error()}
		}
		start := clonePropsTable(props)
		setPropNumber(start, "alpha", 0)
		setPropString(start, "ease", "cubicOut")
		end := clonePropsTable(props)
		setPropNumber(end, "alpha", target)
		return helperKeyframes(
			helperKF{at: at, props: start},
			helperKF{at: at + dur, props: end},
		)
	})

	vm.Register("slideIn", func(args ...interpreter.Object) interpreter.Object {
		props, at, dur, err := helperTiming("slideIn", args, 0.45, 5)
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		x, err := requiredTableNumber(props, "x")
		if err != nil {
			return &interpreter.Error{Message: "slideIn: " + err.Error()}
		}
		y, err := requiredTableNumber(props, "y")
		if err != nil {
			return &interpreter.Error{Message: "slideIn: " + err.Error()}
		}
		dir := "left"
		if len(args) >= 4 {
			dir, err = strArg(args[3], "direction")
			if err != nil {
				return &interpreter.Error{Message: "slideIn: " + err.Error()}
			}
		}
		if !contains([]string{"left", "right", "up", "down"}, dir) {
			return &interpreter.Error{Message: fmt.Sprintf("slideIn: unknown direction %q (want left, right, up, or down)", dir)}
		}
		dist := 64.0
		if len(args) == 5 {
			dist, err = numArg(args[4], "distance")
			if err != nil {
				return &interpreter.Error{Message: "slideIn: " + err.Error()}
			}
		}
		if dist <= 0 {
			return &interpreter.Error{Message: "slideIn: distance must be greater than zero"}
		}

		start := clonePropsTable(props)
		switch dir {
		case "left":
			setPropNumber(start, "x", x-dist)
		case "right":
			setPropNumber(start, "x", x+dist)
		case "up":
			setPropNumber(start, "y", y-dist)
		case "down":
			setPropNumber(start, "y", y+dist)
		}
		setPropString(start, "ease", "cubicOut")
		end := clonePropsTable(props)
		setPropNumber(end, "x", x)
		setPropNumber(end, "y", y)
		return helperKeyframes(
			helperKF{at: at, props: start},
			helperKF{at: at + dur, props: end},
		)
	})

	vm.Register("popIn", func(args ...interpreter.Object) interpreter.Object {
		props, at, dur, err := helperTiming("popIn", args, 0.45, 3)
		if err != nil {
			return &interpreter.Error{Message: err.Error()}
		}
		targetScale, err := tableNumber(props, "scale", 1)
		if err != nil {
			return &interpreter.Error{Message: "popIn: " + err.Error()}
		}
		targetAlpha, err := tableNumber(props, "alpha", 1)
		if err != nil {
			return &interpreter.Error{Message: "popIn: " + err.Error()}
		}

		start := clonePropsTable(props)
		setPropNumber(start, "scale", targetScale*0.80)
		setPropNumber(start, "alpha", 0)
		setPropString(start, "ease", "cubicOut")

		peak := clonePropsTable(props)
		setPropNumber(peak, "scale", targetScale*1.05)
		setPropNumber(peak, "alpha", targetAlpha)
		setPropString(peak, "ease", "cubicInOut")

		end := clonePropsTable(props)
		setPropNumber(end, "scale", targetScale)
		setPropNumber(end, "alpha", targetAlpha)
		return helperKeyframes(
			helperKF{at: at, props: start},
			helperKF{at: at + dur*0.65, props: peak},
			helperKF{at: at + dur, props: end},
		)
	})
}

type helperKF struct {
	at    float64
	props *interpreter.Table
}

// helperKeyframes turns helper output into the table{"0": table{...}}
// shape expected by shape()/text()/group().
func helperKeyframes(kfs ...helperKF) *interpreter.Table {
	pairs := make(map[string]interpreter.Object, len(kfs))
	for _, kf := range kfs {
		pairs["STRING:"+strconv.FormatFloat(kf.at, 'g', -1, 64)] = kf.props
	}
	return &interpreter.Table{Pairs: pairs}
}

func helperTiming(name string, args []interpreter.Object, defDur float64, maxArgs int) (*interpreter.Table, float64, float64, error) {
	if len(args) < 1 || len(args) > maxArgs {
		return nil, 0, 0, fmt.Errorf("%s: expected a props table plus optional start time and duration, got %d arguments", name, len(args))
	}
	props, ok := args[0].(*interpreter.Table)
	if !ok {
		return nil, 0, 0, fmt.Errorf("%s: first argument must be a props table", name)
	}
	at := 0.0
	if len(args) >= 2 {
		v, err := numArg(args[1], "start time")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("%s: %w", name, err)
		}
		at = v
	}
	dur := defDur
	if len(args) >= 3 {
		v, err := numArg(args[2], "duration")
		if err != nil {
			return nil, 0, 0, fmt.Errorf("%s: %w", name, err)
		}
		dur = v
	}
	if at < 0 {
		return nil, 0, 0, fmt.Errorf("%s: start time must not be negative", name)
	}
	if dur <= 0 {
		return nil, 0, 0, fmt.Errorf("%s: duration must be greater than zero", name)
	}
	return props, at, dur, nil
}

func clonePropsTable(t *interpreter.Table) *interpreter.Table {
	pairs := make(map[string]interpreter.Object, len(t.Pairs))
	for k, v := range t.Pairs {
		pairs[k] = v
	}
	return &interpreter.Table{Pairs: pairs}
}

func tableNumber(t *interpreter.Table, key string, def float64) (float64, error) {
	obj, ok := t.Pairs["STRING:"+key]
	if !ok {
		return def, nil
	}
	v, ok := toFloat(interpreter.ObjectToGo(obj))
	if !ok {
		return 0, fmt.Errorf("%q must be a number", key)
	}
	return v, nil
}

func requiredTableNumber(t *interpreter.Table, key string) (float64, error) {
	obj, ok := t.Pairs["STRING:"+key]
	if !ok {
		return 0, fmt.Errorf("props must include %q", key)
	}
	v, ok := toFloat(interpreter.ObjectToGo(obj))
	if !ok {
		return 0, fmt.Errorf("%q must be a number", key)
	}
	return v, nil
}

func setPropNumber(t *interpreter.Table, key string, v float64) {
	t.Pairs["STRING:"+key] = &interpreter.Float{Value: v}
}

func setPropString(t *interpreter.Table, key, v string) {
	t.Pairs["STRING:"+key] = &interpreter.String{Value: v}
}
