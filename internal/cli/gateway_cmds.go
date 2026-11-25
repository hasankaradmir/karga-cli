package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
)

func newGatewayCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Configure the host gateway: where it listens and on which listeners",
		Long: "The gateway is the host's single entry point. Its administrator decides where it listens\n" +
			"(set listen, default 127.0.0.1) and on which ports (listeners); packages only attach their\n" +
			"Services to listeners by name and can open nothing themselves.",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show the gateway's health and the listeners and routes it serves",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := a.backend()
			health, err := s.GatewayHealth(cmd.Context())
			if err != nil {
				return err
			}
			routes, err := s.GatewayRoutes(cmd.Context())
			if err != nil {
				return err
			}
			if a.structured() {
				if routes == nil { // inside an object: write only turns a top-level list into []
					routes = []api.Exposed{}
				}
				return a.write(map[string]any{"gateway": health, "routes": routes})
			}
			state := health.Container
			switch {
			case health.Container == api.ActualMissing:
				state = "not running (nothing is exposed)"
			case health.Ready:
				state += ", ready (Envoy " + health.Version + ")"
			case health.Error != "":
				state += ", admin API unreachable: " + Printable(health.Error)
			default:
				state += ", not ready"
			}
			access := "off"
			if health.AccessLog {
				access = "on (karga logs --gateway)"
			}
			_, _ = fmt.Fprintf(a.stdout, "Gateway:    %s\n", state)
			if health.Image != "" {
				_, _ = fmt.Fprintf(a.stdout, "Image:      %s\n", health.Image)
			}
			_, _ = fmt.Fprintf(a.stdout, "Listen:     %s\n", strings.Join(health.Listen, ", "))
			_, _ = fmt.Fprintf(a.stdout, "Access log: %s\n", access)
			if health.Revision > 0 {
				_, _ = fmt.Fprintf(a.stdout, "Settings:   revision %d (karga gateway history)\n", health.Revision)
			}
			_, _ = fmt.Fprintln(a.stdout)
			var rows [][]string
			for _, r := range routes {
				rows = append(rows, []string{r.Listener, r.Route, r.Release, r.Service, r.Upstream})
			}
			return a.table("LISTENER\tROUTE\tRELEASE\tSERVICE\tUPSTREAM", rows)
		},
	})
	set := &cobra.Command{Use: "set", Short: "Change a gateway setting"}
	set.AddCommand(&cobra.Command{
		Use:       "access-log on|off",
		Short:     "Log every request and connection the gateway handles (off by default)",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"on", "off"},
		RunE: func(cmd *cobra.Command, args []string) error {
			s := a.backend()
			if err := s.SetGatewayAccessLog(cmd.Context(), api.GatewayAccessLogRequest{Enabled: args[0] == "on"}); err != nil {
				return err
			}
			return a.report(map[string]any{"accessLog": args[0] == "on"}, "gateway access log %s\n", args[0])
		},
	})
	set.AddCommand(&cobra.Command{
		Use:   "listen <ip>[,<ip>...]",
		Short: "Set the host addresses every listener binds (default 127.0.0.1; 0.0.0.0 is every interface)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var addrs []string
			for _, ip := range strings.Split(args[0], ",") {
				addrs = append(addrs, strings.TrimSpace(ip))
			}
			req := api.GatewayListenRequest{Addresses: addrs}
			if err := a.confirm(cmd, req.Risk()); err != nil {
				return err
			}
			req.Confirmed = true
			s := a.backend()
			if err := s.SetGatewayListen(cmd.Context(), req); err != nil {
				return err
			}
			return a.report(map[string]any{"listen": addrs}, "gateway listens on %s\n", strings.Join(addrs, ", "))
		},
	})
	cmd.AddCommand(set, newListenerCmd(a))
	addDocumentCmds(a, cmd)
	return cmd
}

// printGatewayPlan shows what applying a gateway document changes; problems make the command fail
// (exit 2).
func (a *app) printGatewayPlan(plan api.GatewayPlan) error {
	if a.structured() {
		if err := a.write(plan); err != nil {
			return err
		}
	} else {
		for _, c := range plan.Changes {
			sign := map[string]string{"add": "+", "remove": "-"}[c.Action]
			if sign == "" {
				sign = "~"
			}
			line := sign + " " + c.Kind
			if c.Name != "" {
				line += " " + c.Name
			}
			if len(c.Fields) > 0 {
				line += ": " + strings.Join(c.Fields, ", ")
			}
			_, _ = fmt.Fprintln(a.stdout, line)
		}
		for _, p := range plan.Problems {
			_, _ = fmt.Fprintln(a.stderr, "problem: "+p)
		}
		if len(plan.Changes) == 0 && len(plan.Problems) == 0 {
			_, _ = fmt.Fprintln(a.stdout, "no changes")
		}
	}
	if len(plan.Problems) > 0 {
		return usageError{fmt.Errorf("the gateway document has %d problem(s); nothing was applied", len(plan.Problems))}
	}
	return nil
}

// optionFlags adds the listener option flags to cmd, written to o.
func optionFlags(cmd *cobra.Command, o *api.ListenerOptions) {
	cmd.Flags().StringVar(&o.Timeout, "timeout", "", "http, https: longest a request may take, e.g. 30s (504 after it; default none)")
	cmd.Flags().StringVar(&o.IdleTimeout, "idle-timeout", "", "close a request or tcp connection idle this long, e.g. 10m (default 5m for http, 1h for tcp)")
	cmd.Flags().StringVar(&o.MaxBody, "max-body", "", "http, https: largest request body, e.g. 10Mi, at most 16Mi (413 beyond it; default none)")
	cmd.Flags().StringVar(&o.RateLimit, "rate-limit", "", "http, https: requests per second or minute, e.g. 100/s (429 beyond it; default none)")
	cmd.Flags().StringVar(&o.ProxyProtocol, "proxy-protocol", "", "tcp: send the client address first, v1 or v2 (the backend must expect it)")
	cmd.Flags().BoolVar(&o.RedirectHTTPS, "redirect-https", false, "http: redirect plain requests for the hosts an https listener serves to it")
}

// unset turns the values that remove an option into none.
func unset(v string) string {
	if v == "0" || v == "off" {
		return ""
	}
	return v
}

// optionsText lists the options that are set, or "-".
func optionsText(o api.ListenerOptions) string {
	var out []string
	redirect := ""
	if o.RedirectHTTPS {
		redirect = "on"
	}
	for _, kv := range [][2]string{{"timeout", o.Timeout}, {"idle-timeout", o.IdleTimeout}, {"max-body", o.MaxBody},
		{"rate-limit", o.RateLimit}, {"proxy-protocol", o.ProxyProtocol}, {"redirect-https", redirect}} {
		if kv[1] != "" {
			out = append(out, kv[0]+"="+kv[1])
		}
	}
	if len(out) == 0 {
		return "-"
	}
	return strings.Join(out, " ")
}

func newListenerCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{Use: "listener", Short: "Define the ports services can be exposed on"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List the listeners and the services using them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			s := a.backend()
			settings, users, err := s.GatewaySettings(cmd.Context())
			if err != nil {
				return err
			}
			if a.structured() {
				return a.write(map[string]any{"listen": settings.Listen, "listeners": settings.Listeners, "users": users})
			}
			var rows [][]string
			for _, l := range settings.Listeners {
				rows = append(rows, []string{l.Name, strconv.Itoa(l.Port), l.Protocol, optionsText(l.ListenerOptions), strings.Join(users[l.Name], ", ")})
			}
			return a.table("NAME\tPORT\tPROTOCOL\tOPTIONS\tSERVICES", rows)
		},
	})
	var l api.Listener
	add := &cobra.Command{
		Use:   "add <name> --port <port> --protocol tcp|http|https",
		Short: "Define a listener (a tcp listener belongs to one service; http and https are shared by host)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			l.Name = args[0]
			s := a.backend()
			if err := s.AddGatewayListener(cmd.Context(), l); err != nil {
				return err
			}
			return a.report(l, "listener %s added (%s on port %d)\n", l.Name, l.Protocol, l.Port)
		},
	}
	add.Flags().IntVar(&l.Port, "port", 0, "host port")
	add.Flags().StringVar(&l.Protocol, "protocol", "", "tcp, http or https")
	_ = add.MarkFlagRequired("port")
	_ = add.MarkFlagRequired("protocol")
	optionFlags(add, &l.ListenerOptions)
	var opts api.ListenerOptions
	set := &cobra.Command{
		Use:   "set <name> [--timeout d] [--idle-timeout d] [--max-body size] [--rate-limit n/s] [--proxy-protocol v1|v2]",
		Short: "Change the options of a listener (0 or off removes one); options not given stay",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s := a.backend()
			settings, _, err := s.GatewaySettings(cmd.Context())
			if err != nil {
				return err
			}
			i := slices.IndexFunc(settings.Listeners, func(l api.Listener) bool { return l.Name == args[0] })
			if i < 0 {
				return fmt.Errorf("gateway listener %q: %w", args[0], api.ErrNotFound)
			}
			req := api.GatewayListenerSetRequest{Name: args[0], ListenerOptions: settings.Listeners[i].ListenerOptions}
			for flag, field := range map[string]*string{"timeout": &req.Timeout, "idle-timeout": &req.IdleTimeout,
				"max-body": &req.MaxBody, "rate-limit": &req.RateLimit, "proxy-protocol": &req.ProxyProtocol} {
				if cmd.Flags().Changed(flag) {
					v, _ := cmd.Flags().GetString(flag)
					*field = unset(v)
				}
			}
			if cmd.Flags().Changed("redirect-https") {
				req.RedirectHTTPS, _ = cmd.Flags().GetBool("redirect-https")
			}
			if err := s.SetGatewayListener(cmd.Context(), req); err != nil {
				return err
			}
			return a.report(req, "listener %s: %s\n", req.Name, optionsText(req.ListenerOptions))
		},
	}
	optionFlags(set, &opts)
	cmd.AddCommand(add, set, &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a listener no service uses",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := api.GatewayListenerRequest{Name: args[0]}
			if err := a.confirm(cmd, req.Risk()); err != nil {
				return err
			}
			req.Confirmed = true
			s := a.backend()
			if err := s.RemoveGatewayListener(cmd.Context(), req); err != nil {
				return err
			}
			return a.report(map[string]any{"listener": args[0], "removed": true}, "listener %s removed\n", args[0])
		},
	})
	return cmd
}
