package main

import (
	"errors"
	"flag"
	"testing"
)

func TestParseHistoryArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		want    map[string]any
	}{
		{"plain", []string{"Task", "t1"}, false, map[string]any{"type_name": "Task", "slug": "t1"}},
		{"flags after", []string{"Task", "t1", "--limit", "5", "--offset", "10", "--view", "gated"}, false,
			map[string]any{"type_name": "Task", "slug": "t1", "limit": 5, "offset": 10, "view": "gated"}},
		{"flags before", []string{"--view", "members", "Decision", "d1"}, false,
			map[string]any{"type_name": "Decision", "slug": "d1", "view": "members"}},
		{"offset zero is sent", []string{"Task", "t1", "--offset", "0"}, false,
			map[string]any{"type_name": "Task", "slug": "t1", "offset": 0}},
		{"bad view", []string{"Task", "t1", "--view", "everyone"}, true, nil},
		{"zero limit", []string{"Task", "t1", "--limit", "0"}, true, nil},
		{"negative offset", []string{"Task", "t1", "--offset", "-3"}, true, nil},
		{"missing slug", []string{"Task"}, true, nil},
		{"nothing", nil, true, nil},
		{"stray argument", []string{"Task", "t1", "extra"}, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseHistoryArgs(tt.args)
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

func TestParseHistoryArgs_Help(t *testing.T) {
	for _, a := range []string{"-h", "--help", "help"} {
		if _, err := parseHistoryArgs([]string{a}); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s: err = %v, want flag.ErrHelp", a, err)
		}
	}
}
