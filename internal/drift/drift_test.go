package drift

import (
	"os"
	"strings"
	"testing"
)

func parseFile(t *testing.T, path string) Report {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	rep, err := Parse(f)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return rep
}

func TestParseDrift(t *testing.T) {
	rep := parseFile(t, "../../testdata/drift.jsonl")
	if !rep.HasDrift() {
		t.Fatal("expected drift")
	}
	// data source read is ignored; 4 real changes remain.
	if got := len(rep.Resources); got != 4 {
		t.Fatalf("want 4 changed resources, got %d: %+v", got, rep.Resources)
	}
	want := map[string]Action{
		"aws_s3_bucket.logs":     Update,
		"aws_security_group.web": Update,
		"aws_instance.app":       Replace, // ["delete","create"] collapses to replace
		"aws_iam_role.dead":      Delete,
	}
	for _, r := range rep.Resources {
		if want[r.Address] != r.Action {
			t.Errorf("%s: want %s, got %s", r.Address, want[r.Address], r.Action)
		}
	}
	if rep.Counts[Update] != 2 || rep.Counts[Replace] != 1 || rep.Counts[Delete] != 1 {
		t.Errorf("bad counts: %+v", rep.Counts)
	}
}

func TestParseClean(t *testing.T) {
	rep := parseFile(t, "../../testdata/clean.jsonl")
	if rep.HasDrift() {
		t.Fatalf("expected no drift, got %+v", rep.Resources)
	}
	if rep.Text() != "no drift" {
		t.Errorf("want 'no drift', got %q", rep.Text())
	}
}

func TestSortedAndDeduped(t *testing.T) {
	in := `{"type":"planned_change","change":{"resource":{"addr":"z.b"},"action":"update"}}
{"type":"planned_change","change":{"resource":{"addr":"a.a"},"action":"create"}}
{"type":"resource_drift","change":{"resource":{"addr":"a.a"},"action":"update"}}`
	rep, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Resources) != 2 {
		t.Fatalf("want 2 deduped resources, got %d", len(rep.Resources))
	}
	if rep.Resources[0].Address != "a.a" || rep.Resources[1].Address != "z.b" {
		t.Errorf("not sorted by address: %+v", rep.Resources)
	}
	// first-seen wins on dedup: a.a stays create.
	if rep.Resources[0].Action != Create {
		t.Errorf("dedup should keep first action; got %s", rep.Resources[0].Action)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		action  string
		actions []string
		want    Action
	}{
		{"create", nil, Create},
		{"update", nil, Update},
		{"delete", nil, Delete},
		{"replace", nil, Replace},
		{"create-then-delete", nil, Replace},
		{"read", nil, Read},
		{"", []string{"delete", "create"}, Replace},
		{"", []string{"create"}, Create},
		{"no-op", nil, NoOp},
		{"", nil, NoOp},
	}
	for _, c := range cases {
		if got := classify(c.action, c.actions); got != c.want {
			t.Errorf("classify(%q,%v)=%s want %s", c.action, c.actions, got, c.want)
		}
	}
}

func TestText(t *testing.T) {
	rep := parseFile(t, "../../testdata/drift.jsonl")
	txt := rep.Text()
	for _, sub := range []string{"drift detected: 4", "aws_instance.app (replace)", "summary:", "2 update"} {
		if !strings.Contains(txt, sub) {
			t.Errorf("text missing %q:\n%s", sub, txt)
		}
	}
}

func TestSlackFormat(t *testing.T) {
	rep := parseFile(t, "../../testdata/drift.jsonl")
	msg := rep.Slack()
	if !strings.Contains(msg, "Terraform drift: 4") || !strings.Contains(msg, "`aws_iam_role.dead`") {
		t.Errorf("bad slack message: %s", msg)
	}
	clean := parseFile(t, "../../testdata/clean.jsonl")
	if !strings.Contains(clean.Slack(), "No Terraform drift") {
		t.Errorf("bad clean slack message: %s", clean.Slack())
	}
}

func TestParseMalformedLinesTolerated(t *testing.T) {
	in := "not json\n{bad json}\n\n{\"type\":\"planned_change\",\"change\":{\"resource\":{\"addr\":\"a.b\"},\"action\":\"create\"}}\n"
	rep, err := Parse(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Resources) != 1 {
		t.Fatalf("want 1 resource past the noise, got %d", len(rep.Resources))
	}
}
