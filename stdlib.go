package kyfram

import (
	"embed"
	"io/fs"
)

//go:embed third_party/std
var stdFiles embed.FS

// StdLib returns the vendored Logos standard library as the FS the VM
// expects: a root containing std/, mirroring the layout the lgs CLI
// embeds. Pass it to NewInterpreter.
func StdLib() fs.FS {
	sub, err := fs.Sub(stdFiles, "third_party")
	if err != nil {
		panic("embedded stdlib missing: " + err.Error())
	}
	return sub
}
