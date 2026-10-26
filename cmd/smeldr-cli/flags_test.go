package main

import (
	"flag"
	"io"
	"reflect"
	"strings"
	"testing"
)

// TestParseInterspersed (T958): flags are read anywhere among the
// positionals; "--" ends them; an unknown flag or a missing value is an
// error, never a positional.
func TestParseInterspersed(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		pos     []string
		reason  string
		json    bool
		wantErr string
	}{
		{"flags before", []string{"--reason", "r", "--json", "a", "b"}, []string{"a", "b"}, "r", true, ""},
		{"flags after", []string{"a", "b", "--reason", "r", "--json"}, []string{"a", "b"}, "r", true, ""},
		{"mixed", []string{"a", "--json", "b", "--reason=r", "c"}, []string{"a", "b", "c"}, "r", true, ""},
		{"repeated value", []string{"--reason", "x", "a", "--reason", "y"}, []string{"a"}, "y", false, ""},
		{"bool at the end", []string{"a", "--json"}, []string{"a"}, "", true, ""},
		{"no flags", []string{"a", "b"}, []string{"a", "b"}, "", false, ""},
		{"nothing", nil, nil, "", false, ""},
		{"double dash", []string{"a", "--json", "--", "-x", "--reason"}, []string{"a", "-x", "--reason"}, "", true, ""},
		{"lone dash", []string{"-", "--json"}, []string{"-"}, "", true, ""},
		{"unknown flag", []string{"a", "--aprove"}, nil, "", false, "flag provided but not defined: -aprove"},
		{"missing value", []string{"a", "--reason"}, nil, "", false, "flag needs an argument: -reason"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("t", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			reason := fs.String("reason", "", "")
			raw := fs.Bool("json", false, "")
			pos, err := parseInterspersed(fs, tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(pos, tc.pos) || *reason != tc.reason || *raw != tc.json {
				t.Errorf("pos %q reason %q json %v err %v; want %q %q %v", pos, *reason, *raw, err, tc.pos, tc.reason, tc.json)
			}
		})
	}
}
