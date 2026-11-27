// Package cli wires karga's cobra commands. It holds no business logic.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
)

// Output formats: text for people (the default; "table" is an older name for it), json and yaml for
// programs. Every command supports all three; streams (--watch, --follow) write one JSON document
// per line, or YAML documents separated by "---".
const (
	outputText  = "text"
	outputTable = "table"
	outputJSON  = "json"
	outputYAML  = "yaml"
)

// app is the per-invocation state shared by all subcommands.
type app struct {
	stdout    io.Writer
	stderr    io.Writer
	output    string
	socket    string
	debug     bool
	logFormat string
	color     string // auto, always or never
	// yes answers confirmations (needed when stdin is not a terminal).
	yes bool
	// requestTimeout bounds commands that only ask karga-server something (see untimed).
	requestTimeout time.Duration
	cancel         context.CancelFunc

	log *slog.Logger
}

// backend is karga-server's client, on --socket (tests serve a fake there).
func (a *app) backend() api.Remote { return api.NewClient(a.socket) }

// NewRootCmd builds the karga command tree.
func NewRootCmd(stdout, stderr io.Writer) *cobra.Command {
	a := &app{stdout: stdout, stderr: stderr}

	root := &cobra.Command{
		Use:               "karga",
		Short:             "Package manager for container workloads",
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error { return a.setup(cmd) },
		PersistentPostRun: func(*cobra.Command, []string) {
			if a.cancel != nil {
				a.cancel()
			}
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)

	addGlobalFlags(a, root)

	root.AddCommand(newVersionCmd(a), newCreateCmd(a), newTemplateCmd(a), newLintCmd(a), newShowCmd(a),
		newInstallCmd(a), newUpgradeCmd(a), newRollbackCmd(a), newUninstallCmd(a),
		newHistoryCmd(a), newListCmd(a), newStatusCmd(a), newGetCmd(a), newGatewayCmd(a), newLogsCmd(a), newVolumeCmd(a), newExecCmd(a), newDiffCmd(a))
	root.AddCommand(newReconcileCmds(a)...)
	addCompletion(a, root)
	for flag, values := range map[string][]string{"output": {outputText, outputJSON, outputYAML}, "color": {"auto", "always", "never"}, "log-format": {"text", "json"}} {
		_ = root.RegisterFlagCompletionFunc(flag, cobra.FixedCompletions(values, cobra.ShellCompDirectiveNoFileComp))
	}
	for path := range untimedCommands {
		if c, _, err := root.Find(strings.Fields(path)[1:]); err != nil || c.CommandPath() != path {
			panic("untimed command that does not exist: " + path) // renamed: every test fails
		}
	}
	markUsageErrors(root)
	markTimedMutations(root)
	return root
}

// setup checks the global flags, sets up logging and the socket, and bounds the command by
// --request-timeout unless it runs as long as its work takes.
func (a *app) setup(cmd *cobra.Command) error {
	switch a.output {
	case outputTable:
		a.output = outputText
	case outputText, outputJSON, outputYAML:
	default:
		return usageError{fmt.Errorf("invalid --output %q: must be text, json or yaml", a.output)}
	}
	switch a.color {
	case "auto", "always", "never":
	default:
		return usageError{fmt.Errorf("invalid --color %q: must be auto, always or never", a.color)}
	}
	level := slog.LevelWarn
	if a.debug {
		level = slog.LevelDebug
	}
	opts := &slog.HandlerOptions{Level: level}
	switch a.logFormat {
	case "text":
		a.log = slog.New(slog.NewTextHandler(a.stderr, opts))
	case "json":
		a.log = slog.New(slog.NewJSONHandler(a.stderr, opts))
	default:
		return usageError{fmt.Errorf("invalid --log-format %q: must be text or json", a.logFormat)}
	}
	if a.socket == "" {
		a.socket = api.DefaultSocket
		if s := os.Getenv("KARGA_SOCKET"); s != "" {
			a.socket = s
		}
	}
	a.log.Debug("karga-server", "socket", a.socket)
	if a.requestTimeout > 0 && !untimed(cmd) {
		ctx, cancel := context.WithTimeout(cmd.Context(), a.requestTimeout)
		cmd.SetContext(ctx)
		a.cancel = cancel
	}
	return nil
}

// addGlobalFlags adds the flags every command takes.
func addGlobalFlags(a *app, root *cobra.Command) {
	root.PersistentFlags().StringVarP(&a.output, "output", "o", outputText, "output format: text|json|yaml")
	root.PersistentFlags().StringVar(&a.socket, "socket", "", "karga-server's socket (default $KARGA_SOCKET or "+api.DefaultSocket+")")
	root.PersistentFlags().DurationVar(&a.requestTimeout, "request-timeout", 30*time.Second,
		"give up waiting for karga-server after this long (0: never); deployments, logs, exec, backups and --watch/--follow are not bounded")
	root.PersistentFlags().BoolVarP(&a.yes, "yes", "y", false, "do not ask before commands that delete data or expose services (needed in scripts)")
	root.PersistentFlags().BoolVar(&a.debug, "debug", false, "enable debug logging")
	root.PersistentFlags().StringVar(&a.logFormat, "log-format", "text", "log format on stderr: text|json")
	root.PersistentFlags().StringVar(&a.color, "color", "auto", "color tables: auto (a terminal without NO_COLOR), always or never")
}

// timedMutations change something and are bounded by --request-timeout, so a timeout can come after
// karga-server applied the change; each says how to see whether it did.
var timedMutations = map[string]string{
	"karga pause": "karga status <release>", "karga resume": "karga status <release>",
	"karga gateway set access-log": "karga gateway get", "karga gateway listener add": "karga gateway listener list",
	"karga gateway listener set": "karga gateway listener list",
}

// markTimedMutations makes a timeout of a timed mutation exit 7 (it may have been applied), not 3.
func markTimedMutations(root *cobra.Command) {
	for path, check := range timedMutations {
		c, _, err := root.Find(strings.Fields(path)[1:])
		if err != nil || c.CommandPath() != path {
			panic("timed mutation that does not exist: " + path)
		}
		run := c.RunE
		c.RunE = func(cmd *cobra.Command, args []string) error {
			err := run(cmd, args)
			if errors.Is(err, context.DeadlineExceeded) {
				return mayHaveApplied{fmt.Errorf("%w: the request timed out and karga-server may have applied it; check with '%s'", api.ErrDetached, check)}
			}
			return err
		}
	}
}

// untimedCommands run as long as their work takes: deployments wait for workloads, others stream,
// open an editor or ask for a confirmation first (the clock must not run meanwhile).
var untimedCommands = map[string]bool{
	"karga install": true, "karga upgrade": true, "karga rollback": true, "karga uninstall": true,
	"karga reconcile": true, "karga logs": true, "karga exec": true,
	"karga volume backup": true, "karga volume restore": true, "karga volume undo-restore": true,
	"karga gateway edit": true, "karga gateway apply": true, "karga gateway rollback": true,
	"karga gateway set listen": true, "karga gateway listener remove": true,
}

// untimed reports whether cmd is left unbounded by --request-timeout.
func untimed(cmd *cobra.Command) bool {
	if untimedCommands[cmd.CommandPath()] {
		return true
	}
	for _, name := range []string{"watch", "follow"} {
		if f := cmd.Flags().Lookup(name); f != nil && f.Value.String() == "true" {
			return true
		}
	}
	return false
}
