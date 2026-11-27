package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"go.yaml.in/yaml/v3"

	"github.com/hasankaradmir/karga-sdk/api"
)

// addDocumentCmds adds the commands that read and apply the gateway settings as a document (kind
// Gateway), kept by the user wherever they like; karga keeps the revisions applied.
func addDocumentCmds(a *app, cmd *cobra.Command) {
	var rev int
	get := &cobra.Command{
		Use:   "get [--revision N]",
		Short: "Print the gateway settings in force (or revision N) as a document to edit and apply",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := a.backend().ExportGateway(cmd.Context(), api.GatewayExportRequest{Revision: rev})
			if err != nil {
				return err
			}
			if a.output == outputJSON {
				var v any
				if err := yaml.Unmarshal(doc.Content, &v); err != nil {
					return err
				}
				return a.write(v)
			}
			_, err = a.stdout.Write(doc.Content)
			return err
		},
	}
	get.Flags().IntVar(&rev, "revision", 0, "revision to print (default: the one in force)")

	var file string
	diff := &cobra.Command{
		Use:   "diff -f <file|->",
		Short: "Show what applying a gateway document changes; changes nothing",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			content, err := readDocument(cmd, file)
			if err != nil {
				return err
			}
			plan, err := a.backend().DiffGateway(cmd.Context(), api.GatewayDocument{Content: content})
			if err != nil {
				return err
			}
			return a.printGatewayPlan(plan)
		},
	}
	diff.Flags().StringVarP(&file, "file", "f", "", "gateway document (- for stdin)")
	_ = diff.MarkFlagRequired("file")

	apply := &cobra.Command{
		Use:   "apply -f <file|->",
		Short: "Apply a gateway document as a new revision; if the gateway does not route with it, the revision in force stays",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			content, err := readDocument(cmd, file)
			if err != nil {
				return err
			}
			return a.applyDocument(cmd, content, "apply")
		},
	}
	apply.Flags().StringVarP(&file, "file", "f", "", "gateway document (- for stdin)")
	_ = apply.MarkFlagRequired("file")

	cmd.AddCommand(get, diff, apply, newGatewayEditCmd(a), &cobra.Command{
		Use:   "history",
		Short: "List the gateway settings revisions kept (the last 10)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			revs, err := a.backend().GatewayHistory(cmd.Context())
			if err != nil {
				return err
			}
			return showTable(a, revs, revs, "REVISION\tTIME\tACTOR\tSOURCE", func(i int, r api.GatewayRevision) []string {
				mark := ""
				if i == len(revs)-1 {
					mark = " (in force)"
				}
				return []string{strconv.Itoa(r.Revision) + mark, when(r.Time), r.Actor, r.Source}
			})
		},
	}, &cobra.Command{
		Use:   "rollback [revision]",
		Short: "Apply an earlier revision of the gateway settings again (default: the one before the one in force)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := a.backend()
			var n int
			if len(args) == 1 {
				var err error
				if n, err = strconv.Atoi(args[0]); err != nil || n < 1 {
					return usageError{fmt.Errorf("revision %q: want a number from karga gateway history", args[0])}
				}
			} else {
				revs, err := s.GatewayHistory(cmd.Context())
				if err != nil {
					return err
				}
				if len(revs) < 2 {
					return fmt.Errorf("no earlier gateway settings revision to roll back to")
				}
				n = revs[len(revs)-2].Revision
			}
			doc, err := s.ExportGateway(cmd.Context(), api.GatewayExportRequest{Revision: n})
			if err != nil {
				return err
			}
			return a.applyDocument(cmd, doc.Content, "rollback "+strconv.Itoa(n))
		},
	})
}

// applyDocument shows what a gateway document changes, asks for its risk, and applies it. With -o
// json or yaml it writes one document: the plan, and the revision once applied.
func (a *app) applyDocument(cmd *cobra.Command, content []byte, source string) error {
	s := a.backend()
	plan, err := s.DiffGateway(cmd.Context(), api.GatewayDocument{Content: content})
	if err != nil {
		return err
	}
	if !a.structured() || len(plan.Changes) == 0 || len(plan.Problems) > 0 {
		if err := a.printGatewayPlan(plan); err != nil || len(plan.Changes) == 0 {
			return err
		}
	}
	if err := a.confirm(cmd, plan.Risk); err != nil {
		return err
	}
	rev, err := s.ApplyGateway(cmd.Context(), api.GatewayApplyRequest{GatewayDocument: api.GatewayDocument{Content: content}, Source: source, Confirmed: true})
	if err != nil {
		return err
	}
	if a.structured() {
		return a.write(struct {
			Plan     api.GatewayPlan     `json:"plan"`
			Revision api.GatewayRevision `json:"revision"`
		}{plan, rev})
	}
	_, err = fmt.Fprintf(a.stdout, "gateway settings applied (revision %d)\n", rev.Revision)
	return err
}

// readDocument reads a gateway document from path, or stdin for "-".
func readDocument(cmd *cobra.Command, path string) ([]byte, error) {
	r := cmd.InOrStdin()
	if path != "-" {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer func() { _ = f.Close() }()
		r = f
	}
	data, err := io.ReadAll(io.LimitReader(r, api.MaxGatewayDocument+1))
	if err == nil && len(data) > api.MaxGatewayDocument {
		err = fmt.Errorf("%s: more than %d bytes", path, api.MaxGatewayDocument)
	}
	return data, err
}

// problemPrefix marks the lines edit adds to say what is wrong with the document.
const problemPrefix = "# problem: "

// newGatewayEditCmd edits the gateway settings in force like kubectl edit: in $KARGA_EDITOR or
// $EDITOR (default vi); a document with problems opens again with them on top; saving it
// unchanged gives up.
func newGatewayEditCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "edit",
		Short: "Edit the gateway settings in force in your editor, then apply them as a new revision",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := a.backend()
			doc, err := s.ExportGateway(cmd.Context(), api.GatewayExportRequest{})
			if err != nil {
				return err
			}
			f, err := os.CreateTemp("", "karga-gateway-*.yaml")
			if err != nil {
				return err
			}
			path := f.Name()
			_ = f.Close()
			keep := false
			defer func() {
				if !keep {
					_ = os.Remove(path)
				}
			}()
			content := doc.Content
			for {
				if err := os.WriteFile(path, content, 0o600); err != nil {
					return err
				}
				if err := runEditor(cmd, path); err != nil {
					return err
				}
				edited, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if string(edited) == string(content) {
					if string(content) == string(doc.Content) {
						_, _ = fmt.Fprintln(a.stdout, "edit cancelled, no changes")
						return nil
					}
					keep = true
					return fmt.Errorf("edit cancelled; your changes are in %s (karga gateway apply -f %s)", path, path)
				}
				clean := withoutProblems(edited)
				plan, err := s.DiffGateway(cmd.Context(), api.GatewayDocument{Content: clean})
				if err != nil {
					keep = true
					return fmt.Errorf("%w; your changes are in %s", err, path)
				}
				if len(plan.Problems) > 0 {
					content = withProblems(clean, plan.Problems)
					continue
				}
				if err := a.applyDocument(cmd, clean, "edit"); err != nil {
					keep = true
					return fmt.Errorf("%w; your changes are in %s", err, path)
				}
				return nil
			}
		},
	}
}

// runEditor opens path in $KARGA_EDITOR, $EDITOR or vi, on the terminal.
func runEditor(cmd *cobra.Command, path string) error {
	editor := os.Getenv("KARGA_EDITOR")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	args := strings.Fields(editor)
	c := exec.CommandContext(cmd.Context(), args[0], append(args[1:], path)...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, cmd.OutOrStdout(), cmd.ErrOrStderr()
	if err := c.Run(); err != nil {
		return fmt.Errorf("editor %s: %w", editor, err)
	}
	return nil
}

// withProblems puts the problems on top of a document, as comments.
func withProblems(doc []byte, problems []string) []byte {
	var b strings.Builder
	for _, p := range problems {
		b.WriteString(problemPrefix + p + "\n")
	}
	b.WriteString("# Fix them and save, or save without a change to give up.\n")
	b.Write(doc)
	return []byte(b.String())
}

// withoutProblems removes the lines withProblems added.
func withoutProblems(doc []byte) []byte {
	lines := strings.SplitAfter(string(doc), "\n")
	for len(lines) > 0 && (strings.HasPrefix(lines[0], problemPrefix) || strings.HasPrefix(lines[0], "# Fix them and save")) {
		lines = lines[1:]
	}
	return []byte(strings.Join(lines, ""))
}
