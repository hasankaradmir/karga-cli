package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
)

type commonFlags struct {
	dryRun     bool
	verbose    bool
	timeout    time.Duration
	historyMax int
	force      bool
	// deleteVolumes allows an upgrade or rollback to delete volumes with retention delete.
	deleteVolumes bool
}

func addDeleteVolumesFlag(cmd *cobra.Command, c *commonFlags) {
	cmd.Flags().BoolVar(&c.deleteVolumes, "delete-volumes", false, "allow deleting volumes the new revision no longer has and that have retention: delete (their data is lost)")
}

func addCommonFlags(cmd *cobra.Command, c *commonFlags) {
	f := cmd.Flags()
	f.BoolVar(&c.dryRun, "dry-run", false, "show the deployment plan without changing anything")
	f.BoolVarP(&c.verbose, "verbose", "v", false, "show every changed field of the plan")
	f.DurationVar(&c.timeout, "timeout", 0, "how long to wait for workloads to become ready (default: karga-server's, 5m)")
	f.IntVar(&c.historyMax, "history-max", 0, "revisions to keep per release, 0 = unlimited (default: karga-server's, 10)")
	f.BoolVar(&c.force, "force", false, "continue after an interrupted operation whose process is gone")
}

func (c commonFlags) options(cmd *cobra.Command) api.CommonOptions {
	o := api.CommonOptions{DryRun: c.dryRun, Timeout: api.Duration(c.timeout), Force: c.force, DeleteVolumes: c.deleteVolumes}
	if cmd.Flags().Changed("history-max") { // otherwise the server's default applies
		o.HistoryMax = &c.historyMax
	}
	return o
}

// optionsFor returns the request options and, except for JSON output and dry runs, live progress on
// stderr: call done once the operation returns.
func (a *app) optionsFor(cmd *cobra.Command, c commonFlags) (opts api.CommonOptions, progress func(api.Event), done func()) {
	opts = c.options(cmd)
	if a.structured() || c.dryRun {
		return opts, nil, func() {}
	}
	p := newProgressPrinter(a.stderr)
	return opts, p.event, p.finish
}

// printResult reports a mutating command. A failed deployment still has a result worth showing.
func (a *app) printResult(verb string, res *api.Result, verbose bool) error {
	if a.structured() {
		return a.write(res)
	}
	for _, w := range res.Warnings {
		if _, err := fmt.Fprintf(a.stderr, "Warning: %s\n", w); err != nil {
			return err
		}
	}
	switch {
	case res.DryRun && verbose:
		return printPlan(a.stdout, res.Plan, res.Release, res.From, res.Revision)
	case res.DryRun:
		return printPlanSummary(a.stdout, res.Plan, res.Release, res.From, res.Revision)
	case res.NoOp:
		_, err := fmt.Fprintf(a.stdout, "Release %q is already installed with the same package and values (revision %d).\n", res.Release, res.Revision)
		return err
	}
	var changed []string
	for _, c := range res.Plan.Changes {
		if c.Action == api.ActionUnchanged {
			continue
		}
		line := fmt.Sprintf("  %s %s %s", c.Action, strings.ToLower(c.Kind), c.Name)
		if c.Strategy != "" {
			line += " (" + c.Label() + ")"
		}
		if n := c.Notice; n != "" {
			line += " (" + n + ")"
		}
		changed = append(changed, line)
	}
	if _, err := fmt.Fprintf(a.stdout, "Release %q %s (revision %d).\n%s\n", res.Release, verb, res.Revision, strings.Join(changed, "\n")); err != nil {
		return err
	}
	if verbose {
		if err := printPlan(a.stdout, res.Plan, res.Release, res.From, res.Revision); err != nil {
			return err
		}
	}
	if res.Notes != "" {
		_, err := fmt.Fprintf(a.stdout, "\nNOTES:\n%s\n", res.Notes)
		return err
	}
	return nil
}

func newInstallCmd(a *app) *cobra.Command {
	var vals valueOpts
	var common commonFlags
	cmd := &cobra.Command{
		Use:   "install <release> <package>",
		Short: "Install a package as a new release",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := a.backend()
			values, err := vals.input()
			if err != nil {
				return err
			}
			opts, progress, done := a.optionsFor(cmd, common)
			res, err := s.Install(cmd.Context(), api.InstallRequest{
				CommonOptions: opts, Name: args[0], Package: args[1], Values: values,
			}, progress)
			done()
			if err != nil {
				return err
			}
			return a.printResult("installed", res, common.verbose)
		},
	}
	addValueFlags(cmd, &vals)
	addCommonFlags(cmd, &common)
	return cmd
}

func addStrategyFlag(cmd *cobra.Command, strategy *string) {
	cmd.Flags().StringVar(strategy, "strategy", "", "override every workload's updateStrategy: recreate|replace")
}

// validStrategy checks --strategy before the package is packed and sent.
func validStrategy(s string) error {
	if err := api.ValidateStrategy(s); err != nil {
		return usageError{fmt.Errorf("invalid --%w", err)}
	}
	return nil
}

func newUpgradeCmd(a *app) *cobra.Command {
	var vals valueOpts
	var common commonFlags
	var reuse, install bool
	var strategy string
	cmd := &cobra.Command{
		Use:   "upgrade <release> <package>",
		Short: "Upgrade a release to a new package version or new values",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validStrategy(strategy); err != nil {
				return err
			}
			s := a.backend()
			values, err := vals.input()
			if err != nil {
				return err
			}
			opts, progress, done := a.optionsFor(cmd, common)
			req := api.UpgradeRequest{
				CommonOptions: opts, Name: args[0], Package: args[1], Values: values,
				ReuseValues: reuse, Install: install, Strategy: strategy,
			}
			if err := a.confirm(cmd, req.Risk(), &req.Confirmed); err != nil {
				return err
			}
			res, err := s.Upgrade(cmd.Context(), req, progress)
			done()
			if err != nil {
				return err
			}
			return a.printResult("upgraded", res, common.verbose)
		},
	}
	addValueFlags(cmd, &vals)
	addCommonFlags(cmd, &common)
	cmd.Flags().BoolVar(&reuse, "reuse-values", false, "start from the current revision's values")
	cmd.Flags().BoolVarP(&install, "install", "i", false, "install the release if it does not exist")
	addStrategyFlag(cmd, &strategy)
	addDeleteVolumesFlag(cmd, &common)
	return cmd
}

func newRollbackCmd(a *app) *cobra.Command {
	var common commonFlags
	var strategy string
	cmd := &cobra.Command{
		Use:   "rollback <release> [revision]",
		Short: "Re-apply an earlier revision as a new revision",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validStrategy(strategy); err != nil {
				return err
			}
			rev, err := revisionArg(args, 1)
			if err != nil {
				return err
			}
			s := a.backend()
			opts, progress, done := a.optionsFor(cmd, common)
			req := api.RollbackRequest{CommonOptions: opts, Name: args[0], Revision: rev, Strategy: strategy}
			if err := a.confirm(cmd, req.Risk(), &req.Confirmed); err != nil {
				return err
			}
			res, err := s.Rollback(cmd.Context(), req, progress)
			done()
			if err != nil {
				return err
			}
			return a.printResult("rolled back", res, common.verbose)
		},
	}
	addCommonFlags(cmd, &common)
	addStrategyFlag(cmd, &strategy)
	addDeleteVolumesFlag(cmd, &common)
	return cmd
}

func newUninstallCmd(a *app) *cobra.Command {
	var req api.UninstallRequest
	cmd := &cobra.Command{
		Use:   "uninstall <release>",
		Short: "Remove a release (volumes are kept unless --delete-volumes)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req.Name = args[0]
			if err := a.confirm(cmd, req.Risk(), &req.Confirmed); err != nil {
				return err
			}
			s := a.backend()
			res, err := s.Uninstall(cmd.Context(), req)
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(res)
			}
			if req.DryRun {
				return printPlanSummary(a.stdout, res.Plan, res.Release, 0, res.From) // no new revision: the current one
			}
			_, err = fmt.Fprintf(a.stdout, "Release %q uninstalled.\n", req.Name)
			return err
		},
	}
	cmd.Flags().BoolVar(&req.DryRun, "dry-run", false, "show what would be removed without changing anything")
	cmd.Flags().BoolVar(&req.DeleteVolumes, "delete-volumes", false, "also delete volumes (data is lost)")
	cmd.Flags().BoolVar(&req.KeepHistory, "keep-history", false, "keep revision history; the release is marked uninstalled")
	cmd.Flags().BoolVar(&req.Force, "force", false, "continue after an interrupted operation whose process is gone")
	return cmd
}
