package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hasankaradmir/karga-sdk/api"
	"go.yaml.in/yaml/v3"
)

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// structured reports whether the output is for programs (json or yaml).
func (a *app) structured() bool { return a.output == outputJSON || a.output == outputYAML }

// write prints v as JSON or YAML, as --output says. An empty list is [], never null.
func (a *app) write(v any) error {
	if rv := reflect.ValueOf(v); rv.Kind() == reflect.Slice && rv.IsNil() {
		v = []any{}
	}
	if a.output == outputYAML {
		return a.writeYAML(v, "")
	}
	return writeJSON(a.stdout, v)
}

// writeStreamed prints one item of a stream: a line of JSON, or a YAML document.
func (a *app) writeStreamed(v any) error {
	if a.output == outputYAML {
		return a.writeYAML(v, "---\n")
	}
	return json.NewEncoder(a.stdout).Encode(v)
}

func (a *app) writeYAML(v any, prefix string) error {
	b, err := toYAMLViaJSON(v)
	if err != nil {
		return err
	}
	_, err = io.WriteString(a.stdout, prefix+string(b))
	return err
}

// toYAMLViaJSON prints v as YAML using its JSON field names (resources only have json tags).
func toYAMLViaJSON(v any) ([]byte, error) {
	js, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(js, &generic); err != nil {
		return nil, err
	}
	return toYAML(generic)
}

// colorful reports whether tables are colored: --color always, or auto on a terminal without
// NO_COLOR (https://no-color.org).
func (a *app) colorful() bool {
	switch a.color {
	case "always":
		return true
	case "never":
		return false
	}
	return !a.structured() && os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb" && isTerminal(a.stdout)
}

// ANSI colors used for state words.
const (
	colorReset  = "\x1b[0m"
	colorRed    = "\x1b[31m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
)

// stateColor picks a color for words karga prints in status columns ("" for none).
func stateColor(cell string) string {
	switch {
	case cell == "":
		return ""
	case strings.Contains(cell, oomKilled) || strings.Contains(cell, api.HealthUnhealthy):
		return colorRed
	}
	// The API's words, and Docker's container states the gateway's health shows (dead, restarting).
	switch strings.SplitN(cell, ",", 2)[0] {
	case api.HealthHealthy, api.ActualRunning, api.StatusDeployed, api.DesiredApplied, api.ActualPresent,
		api.OperationSucceeded, api.ActualRouted:
		return colorGreen
	case api.StatusFailed, api.StatusUnreadable, api.ActualMissing, api.ActualExited, api.ActualNotRouted, "dead", api.OperationAborted:
		return colorRed
	case api.HealthStarting, api.StatusPending, api.StatusDeploying, api.StatusUpgrading, api.StatusUninstalling,
		api.ActualCreated, "restarting":
		return colorYellow
	}
	return ""
}

// stateColumns are the table columns whose words stateColor colors; others (names, descriptions)
// are never colored, whatever they say.
var stateColumns = map[string]bool{"STATUS": true, "DESIRED": true, "ACTUAL": true, "HEALTH/BACKEND": true}

// table writes aligned columns separated by two spaces. Cells are padded before they are colored,
// so color codes never shift the columns.
func (a *app) table(header string, rows [][]string) error {
	head := strings.Split(header, "\t")
	all := append([][]string{head}, rows...)
	for _, r := range all[1:] {
		for i := range r {
			r[i] = Printable(r[i])
		}
	}
	var width []int
	for _, r := range all {
		for i, c := range r {
			if i >= len(width) {
				width = append(width, 0)
			}
			width[i] = max(width[i], utf8.RuneCountInString(c))
		}
	}
	color := a.colorful()
	var b strings.Builder
	for n, r := range all {
		for i, c := range r {
			cell := c
			if i < len(r)-1 {
				cell += strings.Repeat(" ", width[i]-utf8.RuneCountInString(c)+2)
			}
			if code := stateColor(c); color && n > 0 && code != "" && i < len(head) && stateColumns[head[i]] {
				cell = code + c + colorReset + cell[len(c):]
			}
			b.WriteString(cell)
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(a.stdout, b.String())
	return err
}

// Printable shows the control characters of s, text karga-server sent (an error, an event's
// detail), as escapes: an escape sequence in it cannot rewrite or recolor the terminal. Line
// breaks and tabs stay.
func Printable(s string) string {
	if strings.IndexFunc(s, isEscaped) < 0 {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		if isEscaped(r) {
			fmt.Fprintf(&b, "\\x%02x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isEscaped(r rune) bool { return unicode.IsControl(r) && r != '\n' && r != '\t' }

// paint colors one state word when tables are colored.
func (a *app) paint(word string) string {
	if code := stateColor(word); code != "" && a.colorful() {
		return code + word + colorReset
	}
	return word
}

// toYAML marshals v with 2-space indentation and sorted map keys.
func toYAML(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// report ends a command that changed something: v for programs, the formatted line for people.
func (a *app) report(v any, format string, args ...any) error {
	if a.structured() {
		return a.write(v)
	}
	_, err := fmt.Fprintf(a.stdout, format, args...)
	return err
}
