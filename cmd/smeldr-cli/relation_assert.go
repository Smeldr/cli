package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strings"
)

// addressTypes maps a core address prefix to its type, so a bare address
// (D4, ARE1) names its type. A runtime-defined type's address, or anything
// else, is given as Type:ident.
var addressTypes = map[string]string{
	"D": "Decision", "A": "Amendment", "T": "Task", "G": "Goal", "S": "Signal", "R": "Run",
	"P": "task_plan", "U": "Rule", "B": "AuthorityStub", "N": "Subscription",
	"ORG": "Organization", "DOM": "Domain", "ARE": "Area", "SET": "Set",
	"COV": "CoverageRule", "CHK": "ChangeCheck", "IDN": "Identity",
}

var bareAddress = regexp.MustCompile(`^([A-Z]{1,4})([0-9]+)$`)

// parseItemRef reads an item as Type:ident, or as a bare core address whose
// prefix names its type. The server resolves the ident (address, slug or ID).
func parseItemRef(s string) (typeName, ident string, err error) {
	if t, id, ok := strings.Cut(s, ":"); ok {
		if t == "" || id == "" {
			return "", "", fmt.Errorf("%q: give Type:ident, such as Decision:D4", s)
		}
		return t, id, nil
	}
	if m := bareAddress.FindStringSubmatch(s); m != nil {
		if t, ok := addressTypes[m[1]]; ok {
			return t, s, nil
		}
	}
	return "", "", fmt.Errorf("%q: give Type:ident, or an address such as D4 or ARE1", s)
}

// placeKinds are the place a decision place puts a Decision in, by flag.
var placeKinds = []struct{ flag, typeName, kind string }{
	{"area", "Area", "belongs_to_area"},
	{"domain", "Domain", "belongs_to_domain"},
	{"organization", "Organization", "belongs_to_organization"},
}

// parseRelationAssertArgs reads `relation assert <source> <kind> <target>
// [--json]` into assert_relation's arguments. --rev is taken before.
func parseRelationAssertArgs(args []string) (map[string]any, bool, error) {
	c, err := splitArgs(args, map[string]bool{"json": true})
	if err != nil {
		return nil, false, err
	}
	if err := c.only("relation assert", "json"); err != nil {
		return nil, false, err
	}
	if len(c.pos) != 3 {
		return nil, false, fmt.Errorf("relation assert requires <source> <kind> <target>")
	}
	asJSON, err := jsonFlag(c)
	if err != nil {
		return nil, false, err
	}
	params, err := relationParams(c.pos[0], c.pos[1], c.pos[2])
	return params, asJSON, err
}

// parseDecisionPlaceArgs reads `decision place <decision> (--area a |
// --domain d | --organization o) [--json]`: exactly one place.
func parseDecisionPlaceArgs(args []string) (map[string]any, bool, error) {
	c, err := splitArgs(args, map[string]bool{"json": true})
	if err != nil {
		return nil, false, err
	}
	if err := c.only("decision place", "area", "domain", "organization", "json"); err != nil {
		return nil, false, err
	}
	decision, err := c.one("decision place", "decision")
	if err != nil {
		return nil, false, err
	}
	var params map[string]any
	for _, p := range placeKinds {
		v, ok := c.flags[p.flag]
		if !ok {
			continue
		}
		if params != nil {
			return nil, false, fmt.Errorf("decision place takes one place: --area, --domain or --organization")
		}
		if v == "" {
			return nil, false, fmt.Errorf("--%s needs a value", p.flag)
		}
		params = map[string]any{"source_type": "Decision", "source_id": decision,
			"relation_kind": p.kind, "target_type": p.typeName, "target_id": v}
	}
	if params == nil {
		return nil, false, fmt.Errorf("decision place needs a place: --area, --domain or --organization")
	}
	asJSON, err := jsonFlag(c)
	return params, asJSON, err
}

// jsonFlag reads --json: absent is false.
func jsonFlag(c cmdArgs) (bool, error) {
	v, ok := c.flags["json"]
	if !ok {
		return false, nil
	}
	return boolFlag("json", v)
}

// relationParams is assert_relation's arguments for source, kind and target.
func relationParams(source, kind, target string) (map[string]any, error) {
	st, sid, err := parseItemRef(source)
	if err != nil {
		return nil, fmt.Errorf("source %w", err)
	}
	tt, tid, err := parseItemRef(target)
	if err != nil {
		return nil, fmt.Errorf("target %w", err)
	}
	if kind == "" {
		return nil, fmt.Errorf("relation assert requires a kind, such as grounded_in")
	}
	return map[string]any{"source_type": st, "source_id": sid, "relation_kind": kind, "target_type": tt, "target_id": tid}, nil
}

// runRelationAssert asserts one relation over assert_relation.
func runRelationAssert(args []string) {
	runAssertCommand(args, relationAssertUsage, parseRelationAssertArgs)
}

// runDecisionPlace places a Decision over assert_relation.
func runDecisionPlace(args []string) {
	runAssertCommand(args, decisionPlaceUsage, parseDecisionPlaceArgs)
}

// runAssertCommand parses one assert, sends it (with --rev when given) and
// prints the edge, or its JSON with --json.
func runAssertCommand(args []string, usage string, parse func([]string) (map[string]any, bool, error)) {
	args, rev, err := takeRev(args)
	if err != nil {
		fatal("%v", err)
		return
	}
	params, asJSON, err := parse(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Print(usage)
		return
	}
	if err != nil {
		fatal("%v", err)
		return
	}
	if rev != nil {
		params["rev"] = *rev
	}
	cfg, err := loadConfig()
	if err != nil {
		fatal("%v", err)
	}
	text := writeCall(cfg, "assert_relation", params, nil)
	if asJSON {
		if err := printJSON([]byte(text)); err != nil {
			fatal("%v", err)
		}
		return
	}
	fmt.Println(assertedLine(params, text))
}

// assertedLine is the one line a successful assert prints.
func assertedLine(params map[string]any, text string) string {
	var edge struct {
		ID  string `json:"id"`
		Rev *int   `json:"rev"`
	}
	_ = json.Unmarshal([]byte(text), &edge)
	line := fmt.Sprintf("%v:%v %v %v:%v asserted", params["source_type"], params["source_id"], params["relation_kind"], params["target_type"], params["target_id"])
	if edge.ID != "" {
		line += " (relation " + edge.ID
		if edge.Rev != nil {
			line += fmt.Sprintf(", rev %d", *edge.Rev)
		}
		line += ")"
	}
	return line
}

const relationAssertUsage = `smeldr-cli relation assert: assert a relation between two items

Usage:
  smeldr-cli relation assert <source> <kind> <target> [--rev N] [--json]

<source> and <target> are Type:ident (Decision:D4, Area:billing) or a core
address (D4, ARE1, DOM2), whose prefix names the type; the server resolves the
ident (address, slug or ID). <kind> is a registered relation kind, such as
grounded_in or contradicts. The relation is asserted (an observed relation is
not made here). Asserting a live relation of a governance kind again needs
--rev, the rev of the relation as you read it (relation list). To place a
Decision, smeldr-cli decision place is shorter. Requires Author role.
`

const decisionPlaceUsage = `smeldr-cli decision place: put a Decision in its place

Usage:
  smeldr-cli decision place <decision> --area <area> [--rev N] [--json]
  smeldr-cli decision place <decision> --domain <domain> [--rev N] [--json]
  smeldr-cli decision place <decision> --organization <org> [--rev N] [--json]

Asserts belongs_to_area, belongs_to_domain or belongs_to_organization from the
Decision to the place (each an address, slug or ID). A Decision is placed
before it is ratified. Placing a Decision that is no longer proposed needs
ratify over its place. Asserting the same place again needs --rev.
`
