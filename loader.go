package kyfram

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// useLine matches a `use "name"` import line.
var useLine = regexp.MustCompile(`(?m)^\s*use\s+"([^"]+)"\s*$`)

// LoadScript reads path plus its local `use "name"` dependencies
// (`name.lgv` beside the script; `std/` imports stay for Logos itself).
// Dependencies inline first, use-lines stripped, each file once — Go-side
// loading so Logos itself is never forked for multi-file scripts.
// A script with no local imports loads byte-identical apart from a
// trailing newline. Import cycles are an error, not infinite recursion.
func LoadScript(path string) (string, error) {
	done := make(map[string]bool)
	stack := make(map[string]bool)
	var load func(string) (string, error)
	load = func(p string) (string, error) {
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		if stack[abs] {
			return "", fmt.Errorf("import cycle at %s", p)
		}
		if done[abs] {
			return "", nil
		}
		stack[abs] = true
		defer delete(stack, abs)
		done[abs] = true

		src, err := os.ReadFile(abs)
		if err != nil {
			return "", err
		}
		var out strings.Builder
		for _, line := range strings.Split(string(src), "\n") {
			m := useLine.FindStringSubmatch(line)
			if m == nil || strings.HasPrefix(m[1], "std/") {
				out.WriteString(line + "\n")
				continue
			}
			dep, err := load(filepath.Join(filepath.Dir(abs), m[1]+".lgv"))
			if err != nil {
				return "", err
			}
			out.WriteString(dep)
		}
		return out.String(), nil
	}
	return load(path)
}
