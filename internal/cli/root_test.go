package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hasankaradmir/karga-sdk/api"
)

// run runs karga with args. Without --socket it points at a socket nothing listens on, so the tests
// never reach a karga-server running on the machine.
func run(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	if !slices.Contains(args, "--socket") {
		args = append([]string{"--socket", filepath.Join(t.TempDir(), "none.sock")}, args...)
	}
	var out, errOut bytes.Buffer
	cmd := NewRootCmd(&out, &errOut)
	cmd.SetArgs(args)
	err = cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestVersionTable(t *testing.T) {
	out, _, err := run(t, "version")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out, "karga dev (commit ") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestVersionJSON(t *testing.T) {
	out, _, err := run(t, "version", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	var v versionReport
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("invalid JSON %q: %v", out, err)
	}
	if v.Version != "dev" || v.GoVersion == "" {
		t.Fatalf("unexpected version info: %+v", v)
	}
}

func TestInvalidLogFormat(t *testing.T) {
	if _, _, err := run(t, "--log-format", "xml", "version"); err == nil {
		t.Fatal("expected error for --log-format xml")
	}
}

func TestDebugLogsToStderr(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "karga-test.sock")
	out, errOut, err := run(t, "--debug", "--socket", sock, "version", "-o", "json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(errOut, "socket="+sock) {
		t.Fatalf("debug log missing on stderr: %q", errOut)
	}
	if strings.Contains(out, "karga-server") && strings.Contains(out, "socket=") {
		t.Fatalf("log leaked to stdout: %q", out)
	}
}

func TestInvalidOutput(t *testing.T) {
	for _, args := range [][]string{{"version", "-o", "xml"}, {"version", "--color", "sometimes"}} {
		if _, _, err := run(t, args...); ExitCode(err) != ExitUsage {
			t.Fatalf("%v: want a usage error, got %v", args, err)
		}
	}
}

func TestVersionAsksTheServer(t *testing.T) {
	e := newCLIEnv(t)
	if out := e.mustRun("version"); !strings.Contains(out, "karga-server dev") {
		t.Fatalf("version:\n%s", out)
	}
	out, _, err := run(t, "--socket", filepath.Join(t.TempDir(), "none.sock"), "version")
	if err != nil || !strings.Contains(out, "karga-server not running") {
		t.Fatalf("without a server: %v\n%s", err, out)
	}
	if _, _, err := run(t, "--socket", filepath.Join(t.TempDir(), "none.sock"), "list"); err == nil || !strings.Contains(err.Error(), "karga-server is not running") {
		t.Fatalf("list without a server: %v", err)
	}
}

func TestRequestTimeout(t *testing.T) {
	e := newCLIEnv(t)
	e.f.slow = 5 * time.Second
	start := time.Now()
	_, err := e.run("list", "--request-timeout", "100ms")
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > 2*time.Second {
		t.Fatalf("list should give up after the timeout: %v after %s", err, time.Since(start))
	}
	cmd := e.root(io.Discard, io.Discard)
	// An editor or a confirmation must not run against the clock (SC-02, SC-03).
	for path, want := range map[string]bool{"install": true, "logs": true, "list": false, "status": false,
		"gateway edit": true, "gateway apply": true, "gateway rollback": true, "gateway set listen": true,
		"gateway listener remove": true, "volume undo-restore": true, "gateway set access-log": false} {
		c, _, err := cmd.Find(strings.Fields(path))
		if err != nil {
			t.Fatal(err)
		}
		if got := untimed(c); got != want {
			t.Errorf("%s: untimed %v, want %v", path, got, want)
		}
	}
}

// Logs are raw lines: asking for structured output is refused instead of ignored.
func TestLogsRefuseStructuredOutput(t *testing.T) {
	e := newCLIEnv(t)
	if _, err := e.run("logs", "shop", "-o", "json"); ExitCode(err) != ExitUsage {
		t.Fatalf("logs -o json: %v", err)
	}
}

// An empty list prints as [] in every command, never null.
func TestEmptyListsAreEmpty(t *testing.T) {
	var b strings.Builder
	a := &app{stdout: &b, output: outputJSON}
	var none []api.Release
	if err := a.write(none); err != nil || strings.TrimSpace(b.String()) != "[]" {
		t.Fatalf("%q %v", b.String(), err)
	}
}
