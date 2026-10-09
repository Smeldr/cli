package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// contentTypeUsage is the help for the content-type commands.
const contentTypeUsage = `smeldr-cli content-type: runtime-defined content types

Usage:
  content-type define --type <name> --fields <file.json> [--url-prefix /p] [--label L]
                                     define a new type (Admin, define-type)
  content-type redefine --type <name> --fields <file.json> [--label L] [--reason R]
                                     change an existing type's schema (Admin, define-type)
  content-type get <name>            print a type's schema (Author)

--fields is a JSON file holding the full field list: an array of
{name, type, required, format, role, description} objects.

A redefinition takes effect at once, for later writes. It may change the label,
a field's role, format and description, make a required field optional and add
optional fields. It refuses removing a field, changing its type, making it
required, a new required field, and any url_prefix change.
--reason is stored with the change. It is free text that people read: never put
a secret in it.

The MCP endpoint is used (SMELDR_MCP_URL).
`

// runContentTypeCommand dispatches the content-type commands.
func runContentTypeCommand(args []string) {
	tool, params, err := parseContentTypeArgs(args, os.ReadFile)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(os.Stdout, contentTypeUsage)
		return
	}
	if err != nil {
		fatal("%v", err)
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text, err := mcpCall(cfg, tool, params)
	if err != nil {
		fatal("%v", err)
	}
	if err := printJSON([]byte(text)); err != nil {
		fatal("%v", err)
	}
}

// parseContentTypeArgs turns the content-type command line into the MCP tool
// to call and its arguments, reading the fields file through readFile. help,
// -h and --help return flag.ErrHelp. Nothing is sent before it returns.
func parseContentTypeArgs(args []string, readFile func(string) ([]byte, error)) (tool string, params map[string]any, err error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("content-type requires: define, redefine or get")
	}
	switch args[0] {
	case "help", "-h", "--help":
		return "", nil, flag.ErrHelp
	case "get":
		if len(args) != 2 || strings.HasPrefix(args[1], "-") {
			if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
				return "", nil, flag.ErrHelp
			}
			return "", nil, fmt.Errorf("content-type get requires one type name")
		}
		return "get_content_type_schema", map[string]any{"type_name": args[1]}, nil
	case "define", "redefine":
	default:
		return "", nil, fmt.Errorf("unknown content-type command %q (define, redefine or get)", args[0])
	}

	verb := args[0]
	fs := flag.NewFlagSet("content-type "+verb, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	typeName := fs.String("type", "", "")
	fieldsFile := fs.String("fields", "", "")
	label := fs.String("label", "", "")
	urlPrefix := fs.String("url-prefix", "", "")
	reason := fs.String("reason", "", "")
	if err := fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return "", nil, flag.ErrHelp
		}
		return "", nil, err
	}
	if fs.NArg() > 0 {
		return "", nil, fmt.Errorf("content-type %s takes no positional arguments, got %q", verb, fs.Arg(0))
	}
	if *typeName == "" || *fieldsFile == "" {
		return "", nil, fmt.Errorf("content-type %s requires --type and --fields", verb)
	}
	if verb == "define" && *reason != "" {
		return "", nil, fmt.Errorf("--reason applies to redefine only")
	}
	if verb == "redefine" && *urlPrefix != "" {
		return "", nil, fmt.Errorf("--url-prefix cannot be changed by a redefinition")
	}
	raw, err := readFile(*fieldsFile)
	if err != nil {
		return "", nil, fmt.Errorf("read %s: %w", *fieldsFile, err)
	}
	var fields []any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", nil, fmt.Errorf("%s must hold a JSON array of field objects: %w", *fieldsFile, err)
	}
	p := map[string]any{"type_name": *typeName, "fields": fields}
	if *label != "" {
		p["label"] = *label
	}
	if verb == "define" {
		if *urlPrefix != "" {
			p["url_prefix"] = *urlPrefix
		}
		return "define_content_type", p, nil
	}
	if *reason != "" {
		p["reason"] = *reason
	}
	return "redefine_content_type", p, nil
}
