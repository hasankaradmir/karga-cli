package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
	"github.com/hasankaradmir/karga-sdk/resource"
)

// oomKilled marks, in the health column, a container whose last run hit its memory limit.
const oomKilled = "oom-killed"

func newStatusCmd(a *app) *cobra.Command {
	var watch bool
	var interval time.Duration
	var count int
	cmd := &cobra.Command{
		Use:   "status <release>",
		Short: "Compare a release's desired state with the runtime",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := a.backend()
			if !watch {
				rep, err := a.printStatus(cmd.Context(), s, args[0])
				if err == nil && !rep.Healthy() {
					return &ExitCodeError{Code: ExitUnhealthy} // the report says why
				}
				return err
			}
			tty := isTerminal(a.stdout)
			for n := 1; ; n++ {
				if a.structured() { // one document per refresh
					rep, err := s.Status(cmd.Context(), api.ReleaseRequest{Name: args[0]})
					if err != nil {
						return err
					}
					if err := a.writeStreamed(rep); err != nil {
						return err
					}
				} else {
					if tty {
						_, _ = io.WriteString(a.stdout, "\x1b[H\x1b[2J") // clear and redraw
					} else if n > 1 {
						_, _ = io.WriteString(a.stdout, "\n")
					}
					_, _ = fmt.Fprintf(a.stdout, "Every %s: karga status %s    %s\n\n", interval, args[0], time.Now().Format("15:04:05"))
					if _, err := a.printStatus(cmd.Context(), s, args[0]); err != nil {
						return err
					}
				}
				if count > 0 && n >= count {
					return nil
				}
				select {
				case <-cmd.Context().Done():
					return nil
				case <-time.After(interval):
				}
			}
		},
	}
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "refresh until interrupted")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "time between refreshes with --watch")
	cmd.Flags().IntVar(&count, "count", 0, "with --watch, stop after this many refreshes (0: until interrupted)")
	return cmd
}

func (a *app) printStatus(ctx context.Context, s api.Backend, name string) (*api.StatusReport, error) {
	rep, err := s.Status(ctx, api.ReleaseRequest{Name: name})
	if err != nil {
		return nil, err
	}
	if a.structured() {
		return rep, a.write(rep)
	}
	r := rep.Release
	_, _ = fmt.Fprintf(a.stdout, "Release:  %s\nStatus:   %s\nRevision: %d\nPackage:  %s %s\nUpdated:  %s\n",
		r.Name, a.paint(string(r.Status)), r.CurrentRevision, r.PackageName, r.PackageVersion, when(r.UpdatedAt))
	if rc := rep.Reconcile; rc != nil {
		line := "never needed"
		if !rc.Last.IsZero() {
			line = when(rc.Last) + ", " + rc.Status
			if rc.Error != "" {
				line += ": " + rc.Error
			}
		}
		if r.Paused {
			line = "paused (karga resume " + r.Name + ")"
		}
		_, _ = fmt.Fprintf(a.stdout, "Repaired: %s\n", line)
	}
	_, _ = fmt.Fprintln(a.stdout)
	var rows [][]string
	for _, x := range rep.Resources {
		health := x.Health
		if x.Backend != "" {
			health = "-> " + x.Backend
		}
		if x.OOMKilled {
			health = strings.TrimPrefix(health+", "+oomKilled, ", ")
		}
		restarts := ""
		if x.Kind == resource.KindWorkload || x.Kind == api.KindProxy {
			restarts = strconv.Itoa(x.Restarts)
		}
		rows = append(rows, []string{x.Kind, x.Name, x.RuntimeName, strconv.Itoa(x.Revision), x.Desired, x.Actual, restarts, health})
	}
	if err := a.table("KIND\tNAME\tRUNTIME NAME\tREVISION\tDESIRED\tACTUAL\tRESTARTS\tHEALTH/BACKEND", rows); err != nil {
		return nil, err
	}
	if len(rep.Orphans) > 0 {
		_, _ = fmt.Fprintf(a.stdout, "\nNot tracked by karga: %s\n", strings.Join(rep.Orphans, ", "))
	}
	if rep.Interrupted != "" {
		_, _ = fmt.Fprintf(a.stdout, "\nWARNING: operation still marked running: %s\n", rep.Interrupted)
	}
	if rc := rep.Reconcile; rc != nil && len(rc.GaveUp) > 0 {
		_, _ = fmt.Fprintf(a.stdout, "\nkarga-server gave up repairing (karga reconcile %s retries):\n", r.Name)
		for _, what := range slices.Sorted(maps.Keys(rc.GaveUp)) {
			_, _ = fmt.Fprintf(a.stdout, "  %s: %s\n", what, rc.GaveUp[what])
		}
	}
	return rep, nil
}

func newReconcileCmds(a *app) []*cobra.Command {
	reconcile := &cobra.Command{
		Use:   "reconcile <release>",
		Short: "Repair a release now: bring what runs back to its current revision",
		Long: "karga-server does this on its own after container events and every 30 seconds. This runs it now\n" +
			"and forgets earlier failed repairs. It never changes the revision or the values.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := a.backend()
			res, err := s.Reconcile(cmd.Context(), api.ReconcileRequest{Name: args[0], Force: true})
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(res)
			}
			switch {
			case res.Skipped != "":
				_, err = fmt.Fprintf(a.stdout, "Release %q not reconciled: %s\n", res.Release, res.Skipped)
			case len(res.Actions) == 0:
				_, err = fmt.Fprintf(a.stdout, "Release %q runs as its revision; nothing to repair.\n", res.Release)
			default:
				_, err = fmt.Fprintf(a.stdout, "Release %q repaired:\n", res.Release)
				for _, st := range res.Actions {
					line := fmt.Sprintf("  %s %s %s: %s", st.Action, strings.ToLower(st.Kind), st.Name, st.Status)
					if st.Error != "" {
						line += " (" + st.Error + ")"
					}
					_, _ = fmt.Fprintln(a.stdout, line)
				}
			}
			return err
		},
	}
	pause := func(use, short string, paused bool) *cobra.Command {
		return &cobra.Command{
			Use:   use + " <release>",
			Short: short,
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				s := a.backend()
				set := s.Resume
				if paused {
					set = s.Pause
				}
				if err := set(cmd.Context(), api.ReleaseRequest{Name: args[0]}); err != nil {
					return err
				}
				return a.report(map[string]any{"release": args[0], "paused": paused},
					"Release %q %sd: karga-server %s repair it.\n", args[0], use, map[bool]string{true: "does not", false: "will"}[paused])
			},
		}
	}
	var limit int
	var follow bool
	events := &cobra.Command{
		Use:   "events [release]",
		Short: "Show what was done to releases, oldest first: commands and karga-server's repairs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			s := a.backend()
			ops, err := s.Events(cmd.Context(), api.EventsRequest{Name: name, Limit: limit})
			if err != nil {
				return err
			}
			slices.Reverse(ops) // oldest first, so a followed stream continues below
			switch {
			case a.structured() && !follow:
				return a.write(ops)
			case a.structured():
				for _, op := range ops {
					if err := a.writeStreamed(op); err != nil {
						return err
					}
				}
			default:
				if err := a.table("TIME\tRELEASE\tOPERATION\tREVISION\tUSER\tSTATUS\tWHAT", eventRows(ops)); err != nil {
					return err
				}
			}
			var last int64
			if len(ops) > 0 {
				last = ops[len(ops)-1].ID
			}
			for follow {
				select {
				case <-cmd.Context().Done():
					return nil
				case <-time.After(2 * time.Second):
				}
				more, err := s.Events(cmd.Context(), api.EventsRequest{Name: name, AfterID: last, Limit: 100})
				if err != nil {
					return err
				}
				slices.Reverse(more)
				if a.structured() {
					for _, op := range more {
						if err := a.writeStreamed(op); err != nil {
							return err
						}
					}
				} else {
					for _, row := range eventRows(more) {
						_, _ = fmt.Fprintln(a.stdout, strings.Join(row, "  "))
					}
				}
				if len(more) > 0 {
					last = more[len(more)-1].ID
				}
			}
			return nil
		},
	}
	events.Flags().IntVarP(&limit, "limit", "n", 20, "how many events")
	events.Flags().BoolVarP(&follow, "follow", "f", false, "keep printing new events")
	return []*cobra.Command{reconcile, pause("pause", "Stop karga-server from repairing a release (for maintenance)", true),
		pause("resume", "Let karga-server repair a release again", false), events}
}

// eventRows renders operations for karga events; running ones show as such.
func eventRows(ops []api.Operation) [][]string {
	var rows [][]string
	for _, op := range ops {
		what := op.Error
		if what == "" {
			var parts []string
			for _, st := range op.Steps {
				parts = append(parts, st.Action+" "+strings.ToLower(st.Kind)+" "+st.Name)
			}
			what = strings.Join(parts, ", ")
		}
		if d := time.Duration(op.Downtime); d > 0 {
			// How long a volume backup or restore kept its workloads stopped.
			what = strings.TrimPrefix(what+", down "+d.Round(100*time.Millisecond).String(), ", ")
		}
		what = truncate(what, 100)
		user := op.Actor
		if user == "" {
			user = "-" // recorded before karga-server knew its clients
		}
		rows = append(rows, []string{when(op.StartedAt), op.Release, op.Type, strconv.Itoa(op.Revision), user, string(op.Status), what})
	}
	return rows
}

// truncate cuts s to n characters (runes), ending it with "..." when it was longer: a table cell
// stays readable; -o json and -o yaml keep the whole text.
func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-3]) + "..."
	}
	return s
}
