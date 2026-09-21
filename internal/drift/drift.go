// Package drift parses `terraform plan -json` streaming output and classifies
// the resource changes it describes into a compact drift report.
package drift

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Action is a single Terraform change action.
type Action string

const (
	Create  Action = "create"
	Update  Action = "update"
	Delete  Action = "delete"
	Replace Action = "replace"
	NoOp    Action = "no-op"
	Read    Action = "read"
)

// resourceDrift is one changed resource in the plan.
type ResourceDrift struct {
	Address string `json:"address"`
	Type    string `json:"type"`
	Action  Action `json:"action"`
}

// Report is the aggregated result of a plan.
type Report struct {
	Resources []ResourceDrift `json:"resources"`
	Counts    map[Action]int  `json:"counts"`
}

// HasDrift reports whether any actionable change was found. Pure reads and
// no-ops do not count as drift.
func (r Report) HasDrift() bool {
	for _, res := range r.Resources {
		switch res.Action {
		case Create, Update, Delete, Replace:
			return true
		}
	}
	return false
}

// planLine is the subset of a `terraform plan -json` log line we care about.
// Terraform emits one JSON object per line; resource_drift and planned_change
// carry a "change" object with an "action" list.
type planLine struct {
	Type   string `json:"type"`
	Change *struct {
		Resource struct {
			Addr         string `json:"addr"`
			ResourceType string `json:"resource_type"`
		} `json:"resource"`
		Action  string   `json:"action"`
		Actions []string `json:"actions"`
	} `json:"change"`
}

// classify collapses Terraform's action encodings into a single Action.
// Terraform uses either a scalar "action" (e.g. "update") or an "actions"
// list; ["delete","create"] and ["create","delete"] both mean replace.
func classify(action string, actions []string) Action {
	if action == "" && len(actions) > 0 {
		if len(actions) >= 2 {
			return Replace
		}
		action = actions[0]
	}
	switch action {
	case "create":
		return Create
	case "update":
		return Update
	case "delete":
		return Delete
	case "replace", "create-then-delete", "delete-then-create":
		return Replace
	case "read":
		return Read
	default:
		return NoOp
	}
}

// Parse reads streaming `terraform plan -json` output and builds a Report.
// Non-JSON lines and unrelated log types are ignored so it survives noise
// on stderr-merged streams.
func Parse(r io.Reader) (Report, error) {
	rep := Report{Counts: map[Action]int{}}
	seen := map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || line[0] != '{' {
			continue
		}
		var pl planLine
		if err := json.Unmarshal([]byte(line), &pl); err != nil {
			continue // tolerate non-conforming lines
		}
		if pl.Change == nil {
			continue
		}
		if pl.Type != "resource_drift" && pl.Type != "planned_change" {
			continue
		}
		addr := pl.Change.Resource.Addr
		if addr == "" || seen[addr] {
			continue
		}
		act := classify(pl.Change.Action, pl.Change.Actions)
		// Only actionable changes are drift; no-ops and data-source reads are noise.
		if act == NoOp || act == Read {
			continue
		}
		seen[addr] = true
		rep.Resources = append(rep.Resources, ResourceDrift{
			Address: addr,
			Type:    pl.Change.Resource.ResourceType,
			Action:  act,
		})
		rep.Counts[act]++
	}
	if err := sc.Err(); err != nil {
		return rep, fmt.Errorf("read plan stream: %w", err)
	}
	sort.Slice(rep.Resources, func(i, j int) bool {
		return rep.Resources[i].Address < rep.Resources[j].Address
	})
	return rep, nil
}

// symbol maps an action to a leading marker for the text report.
func symbol(a Action) string {
	switch a {
	case Create:
		return "+"
	case Update:
		return "~"
	case Delete:
		return "-"
	case Replace:
		return "±"
	default:
		return " "
	}
}

// Text renders a compact human-readable drift report.
func (r Report) Text() string {
	if !r.HasDrift() {
		return "no drift"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "drift detected: %d resource(s)\n", len(r.Resources))
	for _, res := range r.Resources {
		fmt.Fprintf(&b, "  %s %s (%s)\n", symbol(res.Action), res.Address, res.Action)
	}
	order := []Action{Create, Update, Delete, Replace}
	var parts []string
	for _, a := range order {
		if n := r.Counts[a]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, a))
		}
	}
	fmt.Fprintf(&b, "summary: %s", strings.Join(parts, ", "))
	return b.String()
}

// Slack renders the report as a Slack message payload string.
func (r Report) Slack() string {
	if !r.HasDrift() {
		return ":white_check_mark: No Terraform drift detected."
	}
	var b strings.Builder
	fmt.Fprintf(&b, ":rotating_light: *Terraform drift: %d resource(s)*\n", len(r.Resources))
	for _, res := range r.Resources {
		fmt.Fprintf(&b, "%s `%s` — %s\n", symbol(res.Action), res.Address, res.Action)
	}
	return strings.TrimRight(b.String(), "\n")
}
