package cli

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hasankaradmir/karga-sdk/api"
)

var update = flag.Bool("update", false, "rewrite golden files")

// at is a fixed time, so tables with timestamps do not change from run to run.
var at = time.Date(2026, 11, 18, 9, 30, 0, 0, time.UTC)

// TestMain prints timestamps in UTC: the CLI prints them in local time, and the golden files must
// not depend on the machine's zone. It is set before any test starts a server that reads the clock.
func TestMain(m *testing.M) {
	time.Local = time.UTC
	os.Exit(m.Run())
}

// TestGoldenOutput pins the exact text of the human-readable output: column alignment, headers
// and wording. Run go test ./internal/cli -run TestGoldenOutput -update after an intended change.
func TestGoldenOutput(t *testing.T) {
	history := []api.Revision{
		{Release: "r", Number: 1, Status: api.RevisionDeployed, PackageName: "shop", PackageVersion: "1.0.0", Description: "Install complete", CreatedAt: at},
		{Release: "r", Number: 2, Status: api.RevisionDeployed, PackageName: "shop", PackageVersion: "1.1.0", Description: "Upgrade complete", CreatedAt: at.Add(time.Hour)},
	}
	diff := &api.RevisionDiff{Release: "r", From: 1, To: 2,
		Values: []api.FieldChange{{Path: "tag", Old: `"a"`, New: `"b"`}},
		Resources: []api.ResourceDiff{
			{Kind: "Workload", Name: "web", Action: api.ActionUpdate, Fields: []api.FieldChange{{Path: "spec.image", Old: `"nginx:1"`, New: `"nginx:2"`}}},
			{Kind: "Volume", Name: "cache", Action: api.ActionCreate},
		}}
	status := &api.StatusReport{
		Release: api.Release{Name: "r", Status: api.StatusDeployed, CurrentRevision: 3, PackageName: "shop", PackageVersion: "1.0.0", UpdatedAt: at},
		Resources: []api.ResourceStatus{
			{Kind: "Workload", Name: "web", RuntimeName: "r-web", Revision: 3, Desired: "applied", Actual: "running", Health: "healthy", Restarts: 4},
			{Kind: "Workload", Name: "worker", RuntimeName: "r-worker", Revision: 3, Desired: "applied", Actual: "running", Health: "starting"},
			{Kind: "Service", Name: "web", RuntimeName: "r-karga-proxy", Revision: 3, Desired: "applied", Actual: "routed", Backend: "karga-web-r3"},
			{Kind: "Volume", Name: "data", RuntimeName: "r-data", Revision: 2, Desired: "applied", Actual: "present"},
		},
		Reconcile: &api.ReconcileStatus{Last: at, Status: "succeeded"},
	}
	ops := []api.Operation{
		{ID: 1, Release: "r", Type: "install", Revision: 1, Status: "succeeded", Actor: "alice", StartedAt: at},
		{ID: 2, Release: "r", Type: "upgrade", Revision: 2, Status: "failed", Actor: "alice", Error: "workload web did not become ready", StartedAt: at.Add(time.Hour)},
		{ID: 3, Release: "r", Type: "volume-backup", Revision: 2, Status: "succeeded", Actor: "alice", Downtime: api.Duration(4200 * time.Millisecond), StartedAt: at.Add(2 * time.Hour)},
	}

	cases := []struct {
		name  string
		setup func(f *fake)
		args  []string
	}{
		{"install-dry-run", func(f *fake) { r := installed(); r.DryRun = true; f.result = r }, []string{"install", "r", demo, "--dry-run"}},
		{"install-dry-run-verbose", func(f *fake) { r := installed(); r.DryRun = true; f.result = r }, []string{"install", "r", demo, "--dry-run", "--verbose"}},
		{"upgrade", func(f *fake) { f.result = installed() }, []string{"upgrade", "r", demo}},
		{"uninstall-dry-run", func(f *fake) {
			f.result = &api.Result{Release: "r", Revision: 3, From: 3, DryRun: true, Status: api.StatusDeployed, Plan: api.Plan{Changes: []api.Change{
				{Kind: "Workload", Name: "web", Action: api.ActionDelete}, {Kind: "Volume", Name: "data", Action: api.ActionDelete}}}}
		}, []string{"uninstall", "r", "--dry-run"}},
		{"status", func(f *fake) { f.status = status }, []string{"status", "r"}},
		{"list", func(f *fake) {
			f.releases = []api.Release{
				{Name: "web", Status: api.StatusDeployed, CurrentRevision: 2, PackageName: "shop", PackageVersion: "1.1.0", UpdatedAt: at},
				{Name: "db", Status: api.StatusFailed, CurrentRevision: 1, PackageName: "postgres", PackageVersion: "16.4.0", UpdatedAt: at},
			}
		}, []string{"list"}},
		{"history", func(f *fake) { f.history = history }, []string{"history", "r"}},
		{"history-diff", func(f *fake) { f.history, f.diff = history, diff }, []string{"history", "r", "--diff"}},
		{"diff", func(f *fake) { f.diff = diff }, []string{"diff", "r", "1", "2"}},
		{"events", func(f *fake) { f.ops = ops }, []string{"events", "r"}},
		{"gateway-status", func(f *fake) {
			f.health = api.State{Container: "running", Ready: true}
			f.exposed = []api.Exposed{{Listener: "https:443", Route: "shop.example.com/", Release: "r", Service: "web:80", Upstream: "127.0.0.1:41000"}}
		}, []string{"gateway", "status"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newCLIEnv(t)
			c.setup(e.f)
			out, _ := e.run(append(c.args, "--color", "never")...) // some cases exit non-zero by design
			golden := filepath.Join("testdata", "golden", c.name+".txt")
			if *update {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(out), 0o644); err != nil { //nolint:gosec // test fixture
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run go test ./internal/cli -run TestGoldenOutput -update)", err)
			}
			if out != string(want) {
				t.Fatalf("output differs from %s (run with -update if intended)\n--- got:\n%s\n--- want:\n%s", golden, out, want)
			}
		})
	}
}
