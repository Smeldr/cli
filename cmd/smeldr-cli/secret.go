package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// secretSource names the three inputs of one secret (T961): an environment
// variable (recommended), a flag that reads the first line of stdin, and the
// old value flag, which still works but puts the secret in shell history
// and the process list.
type secretSource struct {
	flag      string // value flag, without dashes: "client-secret"
	stdinFlag string // "client-secret-stdin"
	env       string // "SMELDR_SOCIAL_CLIENT_SECRET"
	missing   string // the refusal when none is given
}

// secretFlags registers the value flag and the stdin flag of s on fs.
func (s secretSource) secretFlags(fs *flag.FlagSet, usage string) (value *string, fromStdin *bool) {
	value = fs.String(s.flag, "", usage+" (deprecated: use $"+s.env+" or --"+s.stdinFlag+")")
	fromStdin = fs.Bool(s.stdinFlag, false, "read "+usage+" from the first line of stdin")
	return value, fromStdin
}

// read returns the secret from exactly one of its sources, after fs is
// parsed. Using the value flag prints a warning to stderr. No error or
// warning ever contains the secret.
func (s secretSource) read(fs *flag.FlagSet, value string, fromStdin bool) (string, error) {
	var got []string
	secret := ""
	if setFlags(fs)[s.flag] {
		got = append(got, "--"+s.flag)
		secret = value
	}
	if fromStdin {
		got = append(got, "--"+s.stdinFlag)
		line, err := bufio.NewReader(stdin).ReadString('\n')
		if err != nil && line == "" {
			return "", fmt.Errorf("--%s: nothing on stdin", s.stdinFlag)
		}
		secret = strings.TrimSpace(line)
	}
	if v, ok := os.LookupEnv(s.env); ok {
		got = append(got, "$"+s.env)
		secret = v
	}
	switch {
	case len(got) == 0:
		return "", errors.New(s.missing + " (set $" + s.env + ", or use --" + s.stdinFlag + ")")
	case len(got) > 1:
		return "", fmt.Errorf("give the secret one way, not %s", strings.Join(got, " and "))
	case strings.TrimSpace(secret) == "":
		return "", fmt.Errorf("%s is empty", got[0])
	}
	if got[0] == "--"+s.flag {
		fmt.Fprintf(os.Stderr, "smeldr-cli: warning: --%s puts the secret in shell history and the process list; use $%s or --%s\n", s.flag, s.env, s.stdinFlag)
	}
	return secret, nil
}
