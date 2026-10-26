package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// osExit is os.Exit; a test replaces it to observe an exit without ending
// the test binary.
var osExit = os.Exit

// newCommandFlags is a command's FlagSet: Parse returns its errors, never
// exits, so the caller refuses an unknown flag by name (T958, T959).
func newCommandFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	return fs
}

// parseCommandFlags parses fs's flags anywhere among args and returns
// exactly want positional arguments. -h prints the usage and exits 0; an
// unknown flag, a missing value or a stray argument is refused through
// fatal, and fewer than want arguments fatal with missing (T959).
func parseCommandFlags(fs *flag.FlagSet, args []string, want int, missing string) []string {
	pos, err := parseAtMost(fs, args, want)
	switch {
	case errors.Is(err, flag.ErrHelp):
		osExit(0)
		return nil
	case err != nil:
		fatal("%s: %v", fs.Name(), err)
	case len(pos) < want:
		fatal("%s", missing)
	}
	return pos
}

// parseAtMost is parseInterspersed refusing more than max positional
// arguments; the caller checks for too few, with its own message.
func parseAtMost(fs *flag.FlagSet, args []string, max int) ([]string, error) {
	pos, err := parseInterspersed(fs, args)
	if err != nil {
		return nil, err
	}
	if len(pos) > max {
		return nil, fmt.Errorf("unexpected argument %q", pos[max])
	}
	return pos, nil
}

// parseInterspersed parses fs's flags anywhere among args and returns the
// positional arguments in order: Go's flag package alone stops at the first
// positional, so a flag after one would be read as an argument (T958). "--"
// ends the flags, and everything after it is positional; a lone "-" is
// positional. With fs at flag.ContinueOnError an unknown flag is an error,
// so nothing that looks like a flag is ever left among the positionals.
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		// Parse stops at "--" too and drops it: what follows is positional.
		// (A flag whose value is literally "--" reads the same; no check
		// flag takes such a value.)
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			return append(pos, rest...), nil
		}
		i := 0
		for i < len(rest) && (rest[i] == "-" || !strings.HasPrefix(rest[i], "-")) {
			i++
		}
		pos = append(pos, rest[:i]...)
		if i == len(rest) {
			return pos, nil
		}
		args = rest[i:]
	}
}
