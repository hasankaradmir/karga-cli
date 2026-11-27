package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
	"github.com/hasankaradmir/karga-sdk/api/apitest"
	"github.com/hasankaradmir/karga-sdk/packages"
	"github.com/hasankaradmir/karga-sdk/resource"
)

// fake is a karga-server stand-in: canned answers, and the requests it got. It is served by the
// SDK's real HTTP layer on a unix socket, so the CLI goes through the same client as in production.
// What karga-server does with the requests is tested in karga-server.
type fake struct {
	mu sync.Mutex

	result      *api.Result
	err         error
	events      []api.Event
	releases    []api.Release
	history     []api.Revision
	status      *api.StatusReport
	diff        *api.RevisionDiff
	slow        time.Duration // List takes this long
	applied     []api.AppliedResource
	ops         []api.Operation
	logs        string
	backup      string
	exitCode    int
	exposed     []api.Exposed
	health      api.State
	settings    api.Settings
	gatewayPlan api.GatewayPlan
	gatewayRevs []api.GatewayRevision
	gatewayDiff func(content string) api.GatewayPlan
	users       map[string][]string
	recon       *api.ReconcileResult

	// What the CLI sent.
	install  *api.InstallRequest
	upgrade  *api.UpgradeRequest
	rollback *api.RollbackRequest
	uninst   *api.UninstallRequest
	pkg      *packages.Package // loaded from the archive the CLI sent
	restored string
	// eventsFn answers Events when set (events -f tests).
	eventsFn func(api.EventsRequest) ([]api.Operation, error)
	// backupReq and restoreReq are the last volume requests.
	backupReq  api.VolumeBackupRequest
	restoreReq api.VolumeRestoreRequest
	reconReq   api.ReconcileRequest
	execCmd    []string
	stdin      string
	calls      []string
}

func (f *fake) call(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
}

func (f *fake) deploy(progress func(api.Event)) (*api.Result, error) {
	for _, e := range f.events {
		if progress != nil {
			progress(e)
		}
	}
	return f.result, f.err
}

func (f *fake) Install(_ context.Context, req api.InstallRequest, progress func(api.Event)) (*api.Result, error) {
	f.call("install")
	f.install = &req
	f.pkg, _ = packages.Load(req.Package)
	return f.deploy(progress)
}

func (f *fake) Upgrade(_ context.Context, req api.UpgradeRequest, progress func(api.Event)) (*api.Result, error) {
	f.call("upgrade")
	f.upgrade = &req
	f.pkg, _ = packages.Load(req.Package)
	return f.deploy(progress)
}

func (f *fake) Rollback(_ context.Context, req api.RollbackRequest, progress func(api.Event)) (*api.Result, error) {
	f.call("rollback")
	f.rollback = &req
	return f.deploy(progress)
}

func (f *fake) Uninstall(_ context.Context, req api.UninstallRequest) (*api.Result, error) {
	f.call("uninstall")
	f.uninst = &req
	if f.result != nil {
		return f.result, f.err
	}
	return &api.Result{Release: req.Name, DryRun: req.DryRun, Status: api.StatusUninstalled}, f.err
}

func (f *fake) List(ctx context.Context) ([]api.Release, error) {
	if f.slow > 0 {
		select {
		case <-time.After(f.slow):
		case <-ctx.Done():
		}
	}
	return f.releases, f.err
}

func (f *fake) History(context.Context, api.ReleaseRequest) ([]api.Revision, error) {
	return f.history, f.err
}

func (f *fake) Values(context.Context, api.ValuesRequest) (map[string]any, error) {
	return map[string]any{"password": "<redacted>", "replicas": 2}, f.err
}

func (f *fake) Notes(context.Context, api.RevisionRequest) (string, error) {
	return "notes here", f.err
}

func (f *fake) Manifest(context.Context, api.RevisionRequest) ([]resource.Resource, error) {
	r, err := resource.Decode([]byte("apiVersion: karga.io/v1alpha1\nkind: Network\nmetadata: {name: net}\n"))
	return []resource.Resource{r}, errors.Join(err, f.err)
}

func (f *fake) Status(context.Context, api.ReleaseRequest) (*api.StatusReport, error) {
	return f.status, f.err
}

func (f *fake) Diff(context.Context, api.DiffRequest) (*api.RevisionDiff, error) {
	return f.diff, f.err
}

func (f *fake) Resources(context.Context, api.ReleaseRequest) ([]api.AppliedResource, error) {
	return f.applied, f.err
}

func (f *fake) Logs(_ context.Context, _ api.LogsRequest, w io.Writer) error {
	_, _ = io.WriteString(w, f.logs)
	return f.err
}

func (f *fake) Exec(_ context.Context, req api.ExecRequest) (int, error) {
	opts := req.ExecOptions
	f.execCmd = opts.Cmd
	if opts.Stdin != nil {
		b, _ := io.ReadAll(opts.Stdin)
		f.stdin = string(b)
	}
	_, _ = io.WriteString(opts.Stdout, "out:"+f.stdin)
	if opts.Stderr != nil {
		_, _ = io.WriteString(opts.Stderr, "to stderr")
	}
	return f.exitCode, f.err
}

func (f *fake) BackupVolume(_ context.Context, req api.VolumeBackupRequest, w io.Writer) (*api.VolumeResult, error) {
	_, _ = io.WriteString(w, f.backup)
	f.backupReq = req
	return &api.VolumeResult{Release: req.Release, Volume: req.Volume, Stopped: []string{"db", "app"}, Downtime: api.Duration(1200 * time.Millisecond)}, f.err
}

func (f *fake) RestoreVolume(_ context.Context, req api.VolumeRestoreRequest, r io.Reader, progress func(api.Event)) (*api.VolumeResult, error) {
	b, _ := io.ReadAll(r)
	f.restored, f.restoreReq = string(b), req
	if progress != nil {
		progress(api.Event{Kind: "Workload", Name: "db", Phase: api.PhaseStop, Detail: "stopping r-db"})
	}
	res := &api.VolumeResult{Release: req.Release, Volume: req.Volume, Stopped: []string{"db"}, Downtime: api.Duration(3 * time.Second)}
	if !req.NoRestorePoint {
		res.RestorePoint = "/var/lib/karga/restore-points/r/data.tar.gz"
	}
	return res, f.err
}

func (f *fake) UndoRestoreVolume(_ context.Context, req api.VolumeUndoRestoreRequest, _ func(api.Event)) (*api.VolumeResult, error) {
	f.call("undo-restore " + req.Release + " " + req.Volume)
	return &api.VolumeResult{Release: req.Release, Volume: req.Volume, Stopped: []string{"db"}, Downtime: api.Duration(2 * time.Second)}, f.err
}

func (f *fake) Reconcile(_ context.Context, req api.ReconcileRequest) (*api.ReconcileResult, error) {
	f.call("reconcile")
	f.reconReq = req
	return f.recon, f.err
}

func (f *fake) Pause(context.Context, api.ReleaseRequest) error {
	f.call("pause")
	return f.err
}

func (f *fake) Resume(context.Context, api.ReleaseRequest) error {
	f.call("resume")
	return f.err
}

func (f *fake) Events(_ context.Context, req api.EventsRequest) ([]api.Operation, error) {
	if f.eventsFn != nil {
		return f.eventsFn(req)
	}
	if req.AfterID > 0 {
		return nil, f.err
	}
	return f.ops, f.err
}

func (f *fake) GatewayRoutes(context.Context) ([]api.Exposed, error) { return f.exposed, f.err }

func (f *fake) GatewayHealth(context.Context) (api.State, error) { return f.health, f.err }

func (f *fake) GatewaySettings(context.Context) (api.Settings, map[string][]string, error) {
	return f.settings, f.users, f.err
}

func (f *fake) SetGatewayAccessLog(_ context.Context, req api.GatewayAccessLogRequest) error {
	f.call(map[bool]string{true: "access-log on", false: "access-log off"}[req.Enabled])
	return f.err
}

func (f *fake) SetGatewayListen(_ context.Context, req api.GatewayListenRequest) error {
	f.settings.Listen = req.Addresses
	f.call("listen")
	return f.err
}

func (f *fake) AddGatewayListener(_ context.Context, l api.Listener) error {
	f.call("add " + l.Name)
	return f.err
}

func (f *fake) SetGatewayListener(_ context.Context, req api.GatewayListenerSetRequest) error {
	for i, l := range f.settings.Listeners {
		if l.Name == req.Name {
			f.settings.Listeners[i].ListenerOptions = req.ListenerOptions
		}
	}
	f.call("set " + req.Name)
	return f.err
}

func (f *fake) DiffGateway(_ context.Context, doc api.GatewayDocument) (api.GatewayPlan, error) {
	f.call("diff " + string(doc.Content))
	if f.gatewayDiff != nil {
		return f.gatewayDiff(string(doc.Content)), f.err
	}
	return f.gatewayPlan, f.err
}

func (f *fake) ApplyGateway(_ context.Context, req api.GatewayApplyRequest) (api.GatewayRevision, error) {
	f.call("apply " + req.Source + " " + string(req.Content))
	return api.GatewayRevision{Revision: 7, Source: req.Source}, f.err
}

func (f *fake) ExportGateway(_ context.Context, req api.GatewayExportRequest) (api.GatewayDocument, error) {
	f.call("export " + strconv.Itoa(req.Revision))
	return api.GatewayDocument{Content: []byte("apiVersion: karga.io/v1alpha1\nkind: Gateway\nspec:\n  accessLog: true\n")}, f.err
}

func (f *fake) GatewayHistory(context.Context) ([]api.GatewayRevision, error) {
	return f.gatewayRevs, f.err
}

func (f *fake) RemoveGatewayListener(_ context.Context, req api.GatewayListenerRequest) error {
	f.call("remove " + req.Name)
	return f.err
}

var _ api.Backend = (*fake)(nil)

// cliEnv runs CLI commands against a fake karga-server on a unix socket.
type cliEnv struct {
	t      *testing.T
	f      *fake
	socket string
	stderr string // of the last run
}

func newCLIEnv(t *testing.T) *cliEnv {
	t.Helper()
	f := &fake{}
	return &cliEnv{t: t, f: f, socket: apitest.Serve(t, f)}
}

func (e *cliEnv) root(stdout, stderr io.Writer) *cobra.Command {
	cmd := NewRootCmd(stdout, stderr)
	_ = cmd.PersistentFlags().Set("socket", e.socket)
	return cmd
}

func (e *cliEnv) run(args ...string) (string, error) {
	e.t.Helper()
	var out, errOut bytes.Buffer
	cmd := e.root(&out, &errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	e.stderr = errOut.String()
	return out.String(), err
}

func (e *cliEnv) mustRun(args ...string) string {
	e.t.Helper()
	out, err := e.run(args...)
	if err != nil {
		e.t.Fatalf("karga %v: %v\n%s", args, err, out)
	}
	return out
}
