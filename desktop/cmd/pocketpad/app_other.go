//go:build !darwin || !cgo

package main

import "errors"

// runApp exists on every platform so the command line is uniform; only
// macOS builds have the window implementation.
func runApp(args []string) error {
	return errors.New("the PocketPad app window is macOS-only; use \"pocketpad serve\" on this system")
}
