package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/hasankaradmir/karga-sdk/api"
)

// planCounts returns how many changes create, update and delete resources, and how many are unchanged.
func planCounts(p api.Plan) (create, update, del, unchanged int) {
	for _, c := range p.Changes {
		switch c.Action {
		case api.ActionCreate:
			create++
		case api.ActionUpdate:
			update++
		case api.ActionDelete:
			del++
		default:
			unchanged++
		}
	}
	return
}

var actionSymbol = map[string]string{api.ActionCreate: "+", api.ActionUpdate: "~", api.ActionDelete: "-"}

// planHeader names the release and the revision a plan moves it to (from 0: a first one).
func planHeader(b *strings.Builder, releaseName string, from, to int) {
	fmt.Fprintf(b, "Deployment Plan\n\nRelease:  %s\n", releaseName)
	if from == 0 {
		fmt.Fprintf(b, "Revision: %d\n", to)
	} else {
		fmt.Fprintf(b, "Revision: %d -> %d\n", from, to)
	}
}

// printPlanSummary writes one line per change (+ create, ~ update, - delete; unchanged resources are left
// out) and the counts. printPlan shows the changed fields too.
func printPlanSummary(w io.Writer, p api.Plan, releaseName string, from, to int) error {
	var b strings.Builder
	planHeader(&b, releaseName, from, to)
	b.WriteString("\n")
	for _, c := range p.Changes {
		sym, ok := actionSymbol[c.Action]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "  %s %s %s", sym, strings.ToUpper(c.Kind), c.Name)
		if c.Strategy != "" {
			fmt.Fprintf(&b, " (%s)", c.Label())
		}
		if n := c.Notice; n != "" {
			fmt.Fprintf(&b, " (%s)", n)
		}
		b.WriteString("\n")
	}
	create, update, del, unchanged := planCounts(p)
	if !p.HasChanges() {
		b.WriteString("  No changes.\n")
	}
	fmt.Fprintf(&b, "\n%d to create, %d to update, %d to delete, %d unchanged.", create, update, del, unchanged)
	if update > 0 {
		b.WriteString(" --verbose shows the changed fields.")
	}
	b.WriteString("\n")
	_, err := io.WriteString(w, b.String())
	return err
}

// printPlan writes the plan with every changed field.
func printPlan(w io.Writer, p api.Plan, releaseName string, from, to int) error {
	var b strings.Builder
	planHeader(&b, releaseName, from, to)
	b.WriteString("\nChanges:\n")
	for _, c := range p.Changes {
		if c.Strategy != "" {
			fmt.Fprintf(&b, "\n  %s %s (%s)\n", strings.ToUpper(c.Kind), c.Name, c.Label())
		} else {
			fmt.Fprintf(&b, "\n  %s %s\n", strings.ToUpper(c.Kind), c.Name)
		}
		switch c.Action {
		case api.ActionUpdate:
			for _, f := range c.Fields {
				writeField(&b, "    ", f)
			}
		default:
			if n := c.Notice; n != "" {
				fmt.Fprintf(&b, "    %s (%s)\n", c.Action, n)
			} else {
				fmt.Fprintf(&b, "    %s\n", c.Action)
			}
		}
	}
	if !p.HasChanges() {
		b.WriteString("\nNo changes.\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// writeField writes one changed field: "path: (added) -> new", "path: old -> new",
// "path: old -> (removed)", or "path: (changed)" for a Secret value, which is never shown.
func writeField(b *strings.Builder, indent string, f api.FieldChange) {
	switch {
	case f.Old == "" && f.New == "":
		fmt.Fprintf(b, "%s%s\n", indent, f.Path)
	case f.Old == "" && f.New == "(changed)":
		fmt.Fprintf(b, "%s%s: (changed)\n", indent, f.Path)
	case f.Old == "":
		fmt.Fprintf(b, "%s%s: (added) -> %s\n", indent, f.Path, f.New)
	case f.New == "":
		fmt.Fprintf(b, "%s%s: %s -> (removed)\n", indent, f.Path, f.Old)
	default:
		fmt.Fprintf(b, "%s%s: %s -> %s\n", indent, f.Path, f.Old, f.New)
	}
}
