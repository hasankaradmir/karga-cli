package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
)

func newLogsCmd(a *app) *cobra.Command {
	var req api.LogsRequest
	cmd := &cobra.Command{
		Use:   "logs <release> [workload...]",
		Short: "Show the logs of a release's workloads, its proxy or the gateway",
		Example: "  karga logs shop            # every workload, prefixed by name\n" +
			"  karga logs shop web -f     # follow one workload\n" +
			"  karga logs shop --proxy    # the release proxy (Linux)\n" +
			"  karga logs --gateway       # the host gateway (Linux)",
		Args: func(_ *cobra.Command, args []string) error {
			if req.Gateway != (len(args) == 0) {
				return errors.New("give a release, or --gateway without one")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if a.structured() {
				return usageError{errors.New("logs are the containers' own lines: -o json and -o yaml do not apply")}
			}
			if len(args) > 0 {
				req.Release, req.Workloads = args[0], args[1:]
			}
			s := a.backend()
			return s.Logs(cmd.Context(), req, a.stdout)
		},
	}
	cmd.Flags().IntVar(&req.Tail, "tail", 100, "lines from the end of each log (-1 for all; events counts with -n/--limit)")
	cmd.Flags().BoolVarP(&req.Follow, "follow", "f", false, "keep streaming new lines (-f is --follow here, a values file in install and upgrade)")
	cmd.Flags().BoolVar(&req.Proxy, "proxy", false, "the release proxy instead of workloads")
	cmd.Flags().BoolVar(&req.Gateway, "gateway", false, "the host gateway")
	return cmd
}
