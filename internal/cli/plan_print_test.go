package cli

import (
	"strings"
	"testing"

	"github.com/hasankaradmir/karga-sdk/api"
)

func samplePlan() api.Plan {
	return api.Plan{Changes: []api.Change{
		{Kind: "Network", Name: "net", Action: api.ActionUnchanged},
		{Kind: "Volume", Name: "cache", Action: api.ActionDelete, Notice: "its data is deleted"},
		{Kind: "Workload", Name: "redis", Action: api.ActionUpdate, Strategy: "recreate",
			Fields: []api.FieldChange{{Path: "spec.image", Old: `"redis:7.4"`, New: `"redis:7.2"`}, {Path: "spec.values.password", New: "(changed)"},
				{Path: "spec.env.OLD", Old: `"x"`}, {Path: "spec.env.NEW", New: `"2222"`}}},
	}}
}

func TestPrintPlan(t *testing.T) {
	var b strings.Builder
	if err := printPlan(&b, samplePlan(), "cache", 2, 3); err != nil {
		t.Fatal(err)
	}
	want := `Deployment Plan

Release:  cache
Revision: 2 -> 3

Changes:

  NETWORK net
    unchanged

  VOLUME cache
    delete (its data is deleted)

  WORKLOAD redis (recreate)
    spec.image: "redis:7.4" -> "redis:7.2"
    spec.values.password: (changed)
    spec.env.OLD: "x" -> (removed)
    spec.env.NEW: (added) -> "2222"
`
	if b.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", b.String(), want)
	}
}

func TestPrintPlanSummary(t *testing.T) {
	var b strings.Builder
	if err := printPlanSummary(&b, samplePlan(), "cache", 1, 2); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	for _, want := range []string{"~ WORKLOAD redis (recreate)", "- VOLUME cache", "0 to create, 1 to update, 1 to delete, 1 unchanged."} {
		if !strings.Contains(out, want) {
			t.Errorf("summary misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "NETWORK net") || strings.Contains(out, "spec.image") {
		t.Errorf("summary shows unchanged resources or fields:\n%s", out)
	}
	b.Reset()
	if err := printPlanSummary(&b, api.Plan{}, "cache", 2, 3); err != nil || !strings.Contains(b.String(), "No changes.") {
		t.Fatalf("empty plan: %q %v", b.String(), err)
	}
}
