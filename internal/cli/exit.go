package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
)

// Exit codes, documented in USAGE.md: scripts can tell why karga failed.
const (
	ExitFailure     = 1 // any other failure
	ExitUsage       = 2 // wrong arguments or flags
	ExitUnavailable = 3 // karga-server is not running, does not answer in time or speaks another API
	ExitNotFound    = 4 // the release, revision or resource does not exist
	ExitConflict    = 5 // another operation holds the release, or it exists already
	ExitUnhealthy   = 6 // karga status: the release is not deployed and healthy
	ExitDetached    = 7 // karga stopped waiting (Ctrl-C); the operation goes on in karga-server
)

// ExitCodeError makes karga exit with Code without printing anything: the command said what it
// had to (a report, or the exec'd command's own output).
type ExitCodeError struct{ Code int }

func (e *ExitCodeError) Error() string { return "exit status " + strconv.Itoa(e.Code) }

// usageError marks wrong arguments or flags.
type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

// ExitCode is the exit status for err, the error a command returned (0 for nil).
func ExitCode(err error) int {
	var exit *ExitCodeError
	var usage usageError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exit):
		return exit.Code
	case errors.As(err, &usage), errors.Is(err, api.ErrInvalid): // the server refused the request as given
		return ExitUsage
	case errors.Is(err, api.ErrDetached):
		return ExitDetached
	case errors.Is(err, api.ErrNotRunning), errors.Is(err, api.ErrIncompatible), errors.Is(err, api.ErrDisconnected),
		errors.Is(err, context.DeadlineExceeded):
		return ExitUnavailable
	case errors.Is(err, api.ErrNotFound):
		return ExitNotFound
	case errors.Is(err, api.ErrConflict):
		return ExitConflict
	}
	return ExitFailure
}

// mayHaveApplied is a timed mutation that timed out (ErrDetached, exit 7); it says how to check.
type mayHaveApplied struct{ error }

func (e mayHaveApplied) Unwrap() error { return e.error }

// Hint is what to do after err, if karga knows ("" otherwise).
func Hint(err error) string {
	var applied mayHaveApplied
	if errors.As(err, &applied) {
		return ""
	}
	if errors.Is(err, api.ErrDetached) {
		return "karga events <release> -f follows it; karga status <release> shows where it got"
	}
	return ""
}

// markUsageErrors makes argument and flag errors of every command usage errors. A group of
// commands (gateway, get, ...) given an unknown one fails: cobra would show its help and exit 0.
func markUsageErrors(root *cobra.Command) {
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if c.HasSubCommands() && !c.Runnable() {
			c.Args = func(cmd *cobra.Command, args []string) error {
				if len(args) == 0 {
					return nil
				}
				msg := fmt.Sprintf("unknown command %q for %q", args[0], cmd.CommandPath())
				cmd.SuggestionsMinimumDistance = cmp.Or(cmd.SuggestionsMinimumDistance, 2) // cobra's default
				if s := cmd.SuggestionsFor(args[0]); len(s) > 0 {
					msg += "; did you mean " + strings.Join(s, " or ") + "?"
				}
				return usageError{errors.New(msg)}
			}
			c.RunE = func(cmd *cobra.Command, _ []string) error { return cmd.Help() }
		}
		if args := c.Args; args != nil {
			c.Args = func(cmd *cobra.Command, a []string) error {
				if err := args(cmd, a); err != nil {
					return usageError{err}
				}
				return nil
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
