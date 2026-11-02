//go:build !windows

package main

import "os"

// stdinIsTerminal reports whether stdin is a character device (a terminal).
func stdinIsTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// terminalHint is added to the refusal on Windows only.
const terminalHint = ""
