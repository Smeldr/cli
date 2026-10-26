package main

import (
	"flag"
	"strings"
)

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
