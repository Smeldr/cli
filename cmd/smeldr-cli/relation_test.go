package main

import (
	"errors"
	"flag"
	"testing"
)

func TestParseRelationArgs(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
		want    map[string]any
	}{
		{"reason after", []string{"withdraw", "e1", "--reason", "wrong tag"}, false, map[string]any{"id": "e1", "reason": "wrong tag"}},
		{"reason before", []string{"withdraw", "--reason", "gone", "e1"}, false, map[string]any{"id": "e1", "reason": "gone"}},
		{"no verb", nil, true, nil},
		{"unknown verb", []string{"delete", "e1"}, true, nil},
		{"missing id", []string{"withdraw", "--reason", "r"}, true, nil},
		{"missing reason", []string{"withdraw", "e1"}, true, nil},
		{"stray argument", []string{"withdraw", "e1", "extra", "--reason", "r"}, true, nil},
		{"bad flag", []string{"withdraw", "--nope"}, true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseRelationArgs(tt.args)
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

func TestParseRelationArgs_Help(t *testing.T) {
	for _, a := range []string{"-h", "--help", "help"} {
		if _, err := parseRelationArgs([]string{"withdraw", a}); !errors.Is(err, flag.ErrHelp) {
			t.Errorf("%s: err = %v, want flag.ErrHelp", a, err)
		}
	}
}
