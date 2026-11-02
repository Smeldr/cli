//go:build windows

package main

import (
	"os"
	"syscall"
)

// stdinIsTerminal reports whether stdin is a console. On Windows the NUL
// device is a character device too, so the mode bits cannot tell; a console
// handle is one GetConsoleMode accepts.
func stdinIsTerminal() bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(os.Stdin.Fd()), &mode) == nil
}

// terminalHint is added to the refusal: a terminal that is not a console
// (an older Git Bash window, mintty without ConPTY) looks like a pipe.
const terminalHint = ". If you are at a terminal that cli cannot ask in (an older Git Bash window without ConPTY), pass --rev, run it in Windows Terminal or PowerShell, or prefix it with winpty"
