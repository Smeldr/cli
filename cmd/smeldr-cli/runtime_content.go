package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// compiledVerbs are the verbs of the prefix form (smeldr-cli <prefix> <verb>).
var compiledVerbs = map[string]bool{
	"list": true, "get": true, "create": true, "update": true,
	"publish": true, "unpublish": true, "archive": true, "delete": true,
}

const runtimeContentUsage = `smeldr-cli content <type> <verb>: items of a runtime-defined type

Usage:
  content <type> list [--status s] [--json] [--fields a,b]
                                     every item the token may read, in any state
  content <type> get <slug|id>       one item
  content <type> create --from <file>
                                     a new draft; the front matter holds the fields,
                                     the text below it goes to the field body; the
                                     slug comes from the type's title-role field
  content <type> update <slug|id> --from <file>
                                     change the fields the file names; the rest stay
  content <type> publish|unpublish|archive <slug|id>
                                     change the item's state

<type> is the type's name (content-type define --type), not its URL prefix.
There is no delete: the server has no delete route for runtime items; archive
the item instead.

A type whose name is one of the verbs above is reached with --type:
  content --type <name> <verb> ...

smeldr-cli content <verb> (list, get, create, update, publish, unpublish,
archive, delete) is the prefix form: it reaches a module served at /content,
as before. smeldr-cli /<prefix> <verb> always means a prefix.
`

// runContentWord dispatches smeldr-cli content: the prefix form when the next
// word is a verb (or nothing), so a module served at /content is reached as
// before; otherwise a runtime-defined type by name.
func runContentWord(args []string) {
	switch {
	case len(args) > 0 && args[0] == "--type":
		if len(args) < 2 || args[1] == "" {
			fatal("content --type requires a type name")
		}
		runRuntimeContent(args[1], args[2:])
	case len(args) > 0 && (args[0] == "-h" || args[0] == "--help" || args[0] == "help"):
		fmt.Fprint(os.Stdout, runtimeContentUsage)
	case len(args) == 0 || compiledVerbs[args[0]]:
		runContentCommand("content", args)
	default:
		runRuntimeContent(args[0], args[1:])
	}
}

// runRuntimeContent runs one verb on a runtime-defined type over the server's
// /_content/{type} routes.
func runRuntimeContent(typeName string, args []string) {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, runtimeContentUsage)
		osExit(1)
		return
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, runtimeContentUsage)
	case "list":
		runRuntimeList(typeName, rest)
	case "get":
		runRuntimeGet(typeName, rest)
	case "create":
		runRuntimeCreate(typeName, rest)
	case "update":
		runRuntimeUpdate(typeName, rest)
	case "publish", "unpublish", "archive":
		runRuntimeStatus(typeName, verb, rest)
	case "delete":
		fatal("items of a runtime-defined type cannot be deleted: the server has no delete route; archive the item instead")
	default:
		fatal("unknown verb %q for content %s: use list, get, create, update, publish, unpublish or archive", verb, typeName)
	}
}

// runtimeBase is the admin route of a runtime-defined type.
func runtimeBase(cfg Config, typeName string) string {
	return cfg.ForgeURL + "/_content/" + url.PathEscape(typeName)
}

// runtimeList is the server's answer to GET /_content/{type}.
type runtimeList struct {
	Items []map[string]any `json:"items"`
	Total int              `json:"total"`
}

// fetchRuntimeList reads every item of typeName the token may read, with an
// optional status filter. The route has no cap without per_page, so this is
// every item; a total above what came back is said, never hidden.
func fetchRuntimeList(cfg Config, typeName, status string) runtimeList {
	u := runtimeBase(cfg, typeName)
	if status != "" {
		u += "?status=" + url.QueryEscape(status)
	}
	raw, code, err := request(cfg, http.MethodGet, u, nil)
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	var l runtimeList
	if err := json.Unmarshal(raw, &l); err != nil {
		fatal("decode list: %v", err)
	}
	return l
}

func runRuntimeList(typeName string, args []string) {
	fs := newCommandFlags("list")
	status := fs.String("status", "", "only items in this state")
	jsonFlag := fs.Bool("json", false, "print raw JSON instead of a table")
	fieldsFlag := fs.String("fields", "", "comma-separated custom columns (case-insensitive)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: smeldr-cli content %s list [--status <s>] [--json] [--fields <a,b,c>]\n", typeName)
		fs.PrintDefaults()
	}
	parseCommandFlags(fs, args, 0, "")
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	l := fetchRuntimeList(cfg, typeName, *status)
	if l.Total > len(l.Items) {
		fmt.Fprintf(os.Stderr, "smeldr-cli: the server answered %d of %d items\n", len(l.Items), l.Total)
	}
	items := make([]any, len(l.Items))
	for i, it := range l.Items {
		items[i] = it
	}
	if *jsonFlag {
		out, _ := json.MarshalIndent(items, "", "  ")
		fmt.Println(string(out))
		return
	}
	cols := []string{"slug", "status", "id"}
	if *fieldsFlag != "" {
		cols = splitFields(*fieldsFlag)
	}
	fmt.Print(keyedTable(items, cols, "No items.\n"))
}

// resolveRuntimeItem returns the ID of the item ref names: ref itself when it
// is an item's ID, else the item whose slug is ref, looked up across every
// state.
func resolveRuntimeItem(cfg Config, typeName, ref string) string {
	raw, code, err := request(cfg, http.MethodGet, runtimeBase(cfg, typeName)+"/"+url.PathEscape(ref), nil)
	if err != nil {
		fatal("%v", err)
	}
	if code == http.StatusOK {
		return ref
	}
	if code != http.StatusNotFound {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	l := fetchRuntimeList(cfg, typeName, "")
	for _, it := range l.Items {
		if s, _ := it[findKeyIn(it, "slug")].(string); s == ref {
			if id, _ := it[findKeyIn(it, "id")].(string); id != "" {
				return id
			}
		}
	}
	if l.Total > len(l.Items) {
		fatal("no %s item with slug or ID %q among the %d of %d items the server answered; name it by its ID", typeName, ref, len(l.Items), l.Total)
	}
	fatal("no %s item with slug or ID %q", typeName, ref)
	return ""
}

func runRuntimeGet(typeName string, args []string) {
	fs := newCommandFlags("get")
	fs.Usage = func() { fmt.Fprintf(fs.Output(), "Usage: smeldr-cli content %s get <slug|id>\n", typeName) }
	ref := parseCommandFlags(fs, args, 1, "get requires a slug or ID")[0]
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	id := resolveRuntimeItem(cfg, typeName, ref)
	runtimeSend(cfg, http.MethodGet, runtimeBase(cfg, typeName)+"/"+url.PathEscape(id), nil)
}

// runtimeFields reads a front matter file as the fields of a runtime item:
// the text below the front matter goes to body when the file does not set it.
func runtimeFields(path string) map[string]any {
	fields, body, err := parseFrontmatterFile(path)
	if err != nil {
		fatal("%v", err)
	}
	if strings.TrimSpace(body) != "" && findKeyIn(fields, "body") == "" {
		fields["body"] = body
	}
	return fields
}

func runRuntimeCreate(typeName string, args []string) {
	fs := newCommandFlags("create")
	from := fs.String("from", "", "front matter file (use - for stdin)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: smeldr-cli content %s create --from <file>\n", typeName)
		fs.PrintDefaults()
	}
	parseCommandFlags(fs, args, 0, "")
	if *from == "" {
		fatal("--from <file> is required for create")
	}
	fields := runtimeFields(*from)
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	runtimeSend(cfg, http.MethodPost, runtimeBase(cfg, typeName), fields)
}

func runRuntimeUpdate(typeName string, args []string) {
	fs := newCommandFlags("update")
	from := fs.String("from", "", "front matter file (use - for stdin)")
	fs.Usage = func() {
		fmt.Fprintf(fs.Output(), "Usage: smeldr-cli content %s update <slug|id> --from <file>\n", typeName)
		fs.PrintDefaults()
	}
	ref := parseCommandFlags(fs, args, 1, "update requires a slug or ID")[0]
	if *from == "" {
		fatal("--from <file> is required for update")
	}
	fields := runtimeFields(*from)
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	id := resolveRuntimeItem(cfg, typeName, ref)
	runtimeSend(cfg, http.MethodPatch, runtimeBase(cfg, typeName)+"/"+url.PathEscape(id), fields)
}

func runRuntimeStatus(typeName, verb string, args []string) {
	fs := newCommandFlags(verb)
	fs.Usage = func() { fmt.Fprintf(fs.Output(), "Usage: smeldr-cli content %s %s <slug|id>\n", typeName, verb) }
	ref := parseCommandFlags(fs, args, 1, verb+" requires a slug or ID")[0]
	status := map[string]string{"publish": "published", "unpublish": "draft", "archive": "archived"}[verb]
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	id := resolveRuntimeItem(cfg, typeName, ref)
	runtimeSend(cfg, http.MethodPost, runtimeBase(cfg, typeName)+"/"+url.PathEscape(id)+"/status", map[string]string{"status": status})
}

// runtimeSend makes one request and prints its answer, or the server's
// refusal as it gave it.
func runtimeSend(cfg Config, method, u string, body any) {
	raw, code, err := request(cfg, method, u, body)
	if err != nil {
		fatal("%v", err)
	}
	if code >= 400 {
		fatal("server returned %d: %s", code, strings.TrimSpace(string(raw)))
	}
	if err := printJSON(raw); err != nil {
		fatal("%v", err)
	}
}
