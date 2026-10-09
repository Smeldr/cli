package main

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"
)

func fakeRead(files map[string]string) func(string) ([]byte, error) {
	return func(name string) ([]byte, error) {
		if s, ok := files[name]; ok {
			return []byte(s), nil
		}
		return nil, errors.New("no such file")
	}
}

func TestParseContentTypeArgs(t *testing.T) {
	read := fakeRead(map[string]string{
		"f.json":   `[{"name":"title","type":"string","required":true}]`,
		"bad.json": `{"name":"x"}`,
	})
	fields := []any{map[string]any{"name": "title", "type": "string", "required": true}}
	cases := []struct {
		name   string
		args   []string
		tool   string
		params map[string]any
		err    string // substring; "help" = flag.ErrHelp
	}{
		{"define", []string{"define", "--type", "memo", "--fields", "f.json", "--url-prefix", "/memos", "--label", "Memo"}, "define_content_type",
			map[string]any{"type_name": "memo", "fields": fields, "url_prefix": "/memos", "label": "Memo"}, ""},
		{"redefine", []string{"redefine", "--type", "memo", "--fields", "f.json", "--reason", "add"}, "redefine_content_type",
			map[string]any{"type_name": "memo", "fields": fields, "reason": "add"}, ""},
		{"get", []string{"get", "memo"}, "get_content_type_schema", map[string]any{"type_name": "memo"}, ""},
		{"help", []string{"help"}, "", nil, "help"},
		{"get help", []string{"get", "--help"}, "", nil, "help"},
		{"flag help", []string{"define", "-h"}, "", nil, "help"},
		{"empty", nil, "", nil, "requires: define"},
		{"unknown verb", []string{"drop"}, "", nil, "unknown content-type command"},
		{"get no name", []string{"get"}, "", nil, "one type name"},
		{"missing fields", []string{"define", "--type", "memo"}, "", nil, "requires --type and --fields"},
		{"positional", []string{"define", "memo", "--type", "memo", "--fields", "f.json"}, "", nil, "no positional"},
		{"unknown flag", []string{"define", "--nope", "x"}, "", nil, "flag provided but not defined"},
		{"reason on define", []string{"define", "--type", "m", "--fields", "f.json", "--reason", "r"}, "", nil, "redefine only"},
		{"prefix on redefine", []string{"redefine", "--type", "m", "--fields", "f.json", "--url-prefix", "/p"}, "", nil, "url-prefix cannot be changed"},
		{"missing file", []string{"define", "--type", "m", "--fields", "none.json"}, "", nil, "read none.json"},
		{"not an array", []string{"define", "--type", "m", "--fields", "bad.json"}, "", nil, "JSON array"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tool, params, err := parseContentTypeArgs(c.args, read)
			switch {
			case c.err == "help":
				if !errors.Is(err, flag.ErrHelp) {
					t.Errorf("err = %v; want help", err)
				}
			case c.err != "":
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Errorf("err = %v; want %q", err, c.err)
				}
			default:
				if err != nil || tool != c.tool || !reflect.DeepEqual(params, c.params) {
					t.Errorf("got %q %v %v; want %q %v", tool, params, err, c.tool, c.params)
				}
			}
		})
	}
}
