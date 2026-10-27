//go:build !js || !wasm

package kyfram

import "os"

// statAsset stats a file the renderer would need. OS build: real stat.
func statAsset(path string) error {
	fi, err := os.Stat(path)
	if err != nil || fi.IsDir() {
		return err
	}
	return nil
}
