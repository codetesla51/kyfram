//go:build js && wasm

package kyfram

import "io/fs"

// statAsset on js/wasm never touches the OS filesystem: file calls
// outside stdin/stdout/stderr can hang the runtime waiting on the
// bridge. Local asset paths simply do not exist in the browser.
func statAsset(_ string) error {
	return fs.ErrNotExist
}
