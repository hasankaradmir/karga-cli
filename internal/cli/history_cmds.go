package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
)

// when writes a time in local time with its UTC offset, so it can be compared with logs in UTC
// (the gateway's access log) or from another host.
func when(t time.Time) string {
	if t.IsZero() { // not known (an unreadable release)
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05 -07:00")
}

func newHistoryCmd(a *app) *cobra.Command {
	var withDiff bool
	cmd := &cobra.Command{
		Use:   "history <release>",
		Short: "Show the revisions of a release",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := a.backend()
			revs, err := s.History(cmd.Context(), api.ReleaseRequest{Name: args[0]})
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(revs)
			}
			header := "REVISION\tSTATUS\tPACKAGE\tVERSION\tCREATED\tDESCRIPTION"
			if withDiff {
				header += "\tCHANGES"
			}
			var rows [][]string
			for i, r := range revs {
				row := []string{strconv.Itoa(r.Number), string(r.Status), r.PackageName, r.PackageVersion, when(r.CreatedAt), truncate(r.Description, 60)}
				if withDiff {
					row = append(row, changeSummary(cmd.Context(), s, revs, i))
				}
				rows = append(rows, row)
			}
			return a.table(header, rows)
		},
	}
	cmd.Flags().BoolVar(&withDiff, "diff", false, "add what changed since the previous revision")
	return cmd
}

// changeSummary counts the differences of revision i from the one before it.
func changeSummary(ctx context.Context, s api.Backend, revs []api.Revision, i int) string {
	if i == 0 {
		return "first"
	}
	d, err := s.Diff(ctx, api.DiffRequest{Name: revs[i].Release, From: revs[i-1].Number, To: revs[i].Number})
	if err != nil {
		return "?"
	}
	n := map[string]int{}
	for _, r := range d.Resources {
		n[r.Action]++
	}
	return fmt.Sprintf("+%d ~%d -%d, %d values", n[api.ActionCreate], n[api.ActionUpdate], n[api.ActionDelete], len(d.Values))
}

func newListCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List releases",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := a.backend()
			rels, err := s.List(cmd.Context())
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(rels)
			}
			var rows [][]string
			for _, r := range rels {
				rows = append(rows, []string{r.Name, strconv.Itoa(r.CurrentRevision), string(r.Status), r.PackageName, r.PackageVersion, when(r.UpdatedAt)})
			}
			return a.table("NAME\tREVISION\tSTATUS\tPACKAGE\tVERSION\tUPDATED", rows)
		},
	}
}

// revisionArg is the revision number in args[i], 0 (the current one) when it is not given.
func revisionArg(args []string, i int) (int, error) {
	if len(args) <= i {
		return 0, nil
	}
	n, err := strconv.Atoi(args[i])
	if err != nil || n < 1 {
		return 0, usageError{fmt.Errorf("invalid revision %q: must be a number from 1", args[i])}
	}
	return n, nil
}

func newGetCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "get", Short: "Show stored information about a release"}
	var all bool
	values := &cobra.Command{
		Use:   "values <release> [revision]",
		Short: "Show the values a revision was deployed with (user overrides unless --all)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			revision, err := revisionArg(args, 1)
			if err != nil {
				return err
			}
			s := a.backend()
			v, err := s.Values(cmd.Context(), api.ValuesRequest{Name: args[0], Revision: revision, All: all})
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(v)
			}
			out, err := toYAML(v)
			if err != nil {
				return err
			}
			_, err = a.stdout.Write(out)
			return err
		},
	}
	values.Flags().BoolVar(&all, "all", false, "show the merged values including package defaults")

	manifest := &cobra.Command{
		Use:   "manifest <release> [revision]",
		Short: "Show the resources a revision deployed (secret values redacted)",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			revision, err := revisionArg(args, 1)
			if err != nil {
				return err
			}
			s := a.backend()
			rs, err := s.Manifest(cmd.Context(), api.RevisionRequest{Name: args[0], Revision: revision})
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(rs)
			}
			for _, r := range rs {
				out, err := toYAMLViaJSON(r)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(a.stdout, "---\n%s", out); err != nil {
					return err
				}
			}
			return nil
		},
	}
	notes := &cobra.Command{
		Use:   "notes <release> [revision]",
		Short: "Show the package's notes (NOTES.txt) for a revision",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			revision, err := revisionArg(args, 1)
			if err != nil {
				return err
			}
			s := a.backend()
			n, err := s.Notes(cmd.Context(), api.RevisionRequest{Name: args[0], Revision: revision})
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(map[string]string{"notes": n})
			}
			if n == "" {
				n = "(the package has no NOTES.txt)"
			}
			_, err = fmt.Fprintln(a.stdout, n)
			return err
		},
	}
	cmd.AddCommand(values, manifest, notes)
	return cmd
}

func newDiffCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "diff <release> <revision> [revision]",
		Short: "Show how two revisions differ (the second defaults to the current one)",
		Args:  cobra.RangeArgs(2, 3),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, err := revisionArg(args, 1)
			if err != nil {
				return err
			}
			to, err := revisionArg(args, 2)
			if err != nil {
				return err
			}
			s := a.backend()
			d, err := s.Diff(cmd.Context(), api.DiffRequest{Name: args[0], From: from, To: to})
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(d)
			}
			var b strings.Builder
			fmt.Fprintf(&b, "Release %s: revision %d -> %d\n", d.Release, d.From, d.To)
			fields := func(fs []api.FieldChange, indent string) {
				for _, f := range fs {
					writeField(&b, indent, f)
				}
			}
			if len(d.Package) > 0 {
				b.WriteString("\nPackage:\n")
				fields(d.Package, "  ")
			}
			if len(d.Values) > 0 {
				b.WriteString("\nValues:\n")
				fields(d.Values, "  ")
			}
			b.WriteString("\nResources:\n")
			if len(d.Resources) == 0 {
				b.WriteString("  no differences\n")
			}
			for _, r := range d.Resources {
				fmt.Fprintf(&b, "  %s %s %s\n", actionSymbol[r.Action], strings.ToUpper(r.Kind), r.Name)
				fields(r.Fields, "      ")
			}
			_, err = io.WriteString(a.stdout, b.String())
			return err
		},
	}
}
