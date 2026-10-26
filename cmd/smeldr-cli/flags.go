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
	pos := parseCommandFlagsAtMost(fs, args, want)
	if len(pos) < want {
		fatal("%s", missing)
	}
	return pos
}

// oneArgument is the one argument of a command that takes exactly one and
// no flags: a second argument or any flag is refused (T960).
func oneArgument(name string, args []string, missing string) string {
	return parseCommandFlags(newCommandFlags(name), args, 1, missing)[0]
}

// parseCommandFlagsAtMost is parseCommandFlags for a command whose
// arguments are optional, up to max; the caller checks for too few.
func parseCommandFlagsAtMost(fs *flag.FlagSet, args []string, max int) []string {
	pos, err := parseAtMost(fs, args, max)
	switch {
	case errors.Is(err, flag.ErrHelp):
		osExit(0)
	case err != nil:
		fatal("%s: %v", fs.Name(), err)
	}
	return pos
}

// wantsHelp reports whether args ask for help: -h or --help anywhere, or
// help as the first argument (later, "help" may be a value). Such a command
// prints its help and returns, as it always has.
func wantsHelp(args []string) bool {
	if len(args) > 0 && args[0] == "help" {
		return true
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// setFlags reports which of fs's flags were given, for a command that sends
// a value only when it was set.
func setFlags(fs *flag.FlagSet) map[string]bool {
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
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
