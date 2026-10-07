package main

import (
	"errors"
	"flag"
	"testing"
)

func TestParseReachabilityArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		want    map[string]any
	}{
		{"plain", []string{"post", "p1"}, false, map[string]any{"type_name": "post", "id": "p1"}},
		{"all flags after", []string{"post", "p1", "--kind", "cites", "--direction", "outgoing", "--depth", "3", "--max-items", "50", "--limit", "10", "--offset", "5"}, false,
			map[string]any{"type_name": "post", "id": "p1", "kind": "cites", "direction": "outgoing", "depth": 3, "max_items": 50, "limit": 10, "offset": 5}},
		{"flags before", []string{"--direction", "incoming", "post", "p1"}, false,
			map[string]any{"type_name": "post", "id": "p1", "direction": "incoming"}},
		{"offset zero is sent", []string{"post", "p1", "--offset", "0"}, false,
			map[string]any{"type_name": "post", "id": "p1", "offset": 0}},
		{"source is not a direction", []string{"post", "p1", "--direction", "source"}, true, nil},
		{"depth zero", []string{"post", "p1", "--depth", "0"}, true, nil},
		{"depth eleven", []string{"post", "p1", "--depth", "11"}, true, nil},
		{"max-items too big", []string{"post", "p1", "--max-items", "2001"}, true, nil},
		{"limit zero", []string{"post", "p1", "--limit", "0"}, true, nil},
		{"negative offset", []string{"post", "p1", "--offset", "-1"}, true, nil},
		{"missing id", []string{"post"}, true, nil},
		{"stray argument", []string{"post", "p1", "extra"}, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseReachabilityArgs(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("params = %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("params[%s] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestParseReachabilityArgs_Help(t *testing.T) {
	for _, a := range []string{"-h", "--help", "help"} {
		if _, err := parseReachabilityArgs([]string{a}); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s: err = %v, want flag.ErrHelp", a, err)
		}
	}
}
