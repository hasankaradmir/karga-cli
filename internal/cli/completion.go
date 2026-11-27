package cli

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hasankaradmir/karga-sdk/api"
	"github.com/hasankaradmir/karga-sdk/resource"
)

// Dynamic shell completion asks karga-server, briefly; without a server it offers nothing.

// completer offers values for one argument position; args are the arguments before it.
type completer func(a *app, args []string) ([]string, cobra.ShellCompDirective)

func (a *app) ask(fn func(context.Context, api.Backend) []string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return fn(ctx, a.backend())
}

func completeRelease(a *app, _ []string) ([]string, cobra.ShellCompDirective) {
	return a.ask(func(ctx context.Context, b api.Backend) []string {
		rels, _ := b.List(ctx)
		var out []string
		for _, r := range rels {
			out = append(out, r.Name)
		}
		return out
	}), cobra.ShellCompDirectiveNoFileComp
}

// completeResource offers the applied resources of kind of the release in args[0].
func completeResource(kind string) completer {
	return func(a *app, args []string) ([]string, cobra.ShellCompDirective) {
		return a.ask(func(ctx context.Context, b api.Backend) []string {
			applied, _ := b.Resources(ctx, api.ReleaseRequest{Name: args[0]})
			var out []string
			for _, r := range applied {
				if r.Kind == kind {
					out = append(out, r.Name)
				}
			}
			return out
		}), cobra.ShellCompDirectiveNoFileComp
	}
}

func completeRevision(a *app, args []string) ([]string, cobra.ShellCompDirective) {
	return a.ask(func(ctx context.Context, b api.Backend) []string {
		revs, _ := b.History(ctx, api.ReleaseRequest{Name: args[0]})
		var out []string
		for i := len(revs) - 1; i >= 0; i-- {
			out = append(out, strconv.Itoa(revs[i].Number)+"\t"+string(revs[i].Status)+" "+revs[i].PackageVersion)
		}
		return out
	}), cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
}

func completeListener(a *app, _ []string) ([]string, cobra.ShellCompDirective) {
	return a.ask(func(ctx context.Context, b api.Backend) []string {
		settings, _, _ := b.GatewaySettings(ctx)
		var out []string
		for _, l := range settings.Listeners {
			out = append(out, l.Name+"\t"+l.Protocol+" "+strconv.Itoa(l.Port))
		}
		return out
	}), cobra.ShellCompDirectiveNoFileComp
}

// completeGatewayRevision offers the gateway settings revisions kept, newest first.
func completeGatewayRevision(a *app, _ []string) ([]string, cobra.ShellCompDirective) {
	return a.ask(func(ctx context.Context, b api.Backend) []string {
		revs, _ := b.GatewayHistory(ctx)
		var out []string
		for i := len(revs) - 1; i >= 0; i-- {
			out = append(out, strconv.Itoa(revs[i].Revision)+"\t"+revs[i].Source)
		}
		return out
	}), cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
}

func completeDir(*app, []string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveFilterDirs
}

func completeFile(*app, []string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveDefault
}

func completeNothing(*app, []string) ([]string, cobra.ShellCompDirective) {
	return nil, cobra.ShellCompDirectiveNoFileComp
}

// positional completes argument i with byPos[i]; the last completer repeats when repeat is set.
func positional(a *app, repeat bool, byPos ...completer) func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
	return func(_ *cobra.Command, args []string, prefix string) ([]string, cobra.ShellCompDirective) {
		i := len(args)
		if i >= len(byPos) {
			if !repeat {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			i = len(byPos) - 1
		}
		vals, dir := byPos[i](a, args)
		var out []string
		for _, v := range vals {
			if strings.HasPrefix(v, prefix) {
				out = append(out, v)
			}
		}
		return out, dir
	}
}

// addCompletion attaches dynamic completion to the command tree.
func addCompletion(a *app, root *cobra.Command) {
	set := func(path string, repeat bool, byPos ...completer) {
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil || cmd == root || cmd.CommandPath() != "karga "+path {
			panic("completion for a command that does not exist: " + path) // renamed: every test fails
		}
		cmd.ValidArgsFunction = positional(a, repeat, byPos...)
	}
	workload := completeResource(resource.KindWorkload)
	volume := completeResource(resource.KindVolume)
	set("install", false, completeNothing, completeDir)
	set("upgrade", false, completeRelease, completeDir)
	set("template", false, completeNothing, completeDir)
	set("lint", true, completeDir)
	set("create", false, completeDir)
	set("rollback", false, completeRelease, completeRevision)
	set("diff", false, completeRelease, completeRevision, completeRevision)
	for _, c := range []string{"uninstall", "history", "status", "get values", "get manifest", "get notes", "reconcile", "pause", "resume", "events"} {
		set(c, false, completeRelease)
	}
	set("logs", true, completeRelease, workload)
	set("exec", false, completeRelease, workload)
	set("volume backup", false, completeRelease, volume, completeFile)
	set("volume restore", false, completeRelease, volume, completeFile)
	set("volume undo-restore", false, completeRelease, volume)
	set("gateway listener set", false, completeListener)
	set("gateway listener remove", false, completeListener)
	set("gateway rollback", false, completeGatewayRevision)
	flag := func(path, name string, fn func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective)) {
		cmd, _, err := root.Find(strings.Fields(path))
		if err != nil || cmd.CommandPath() != "karga "+path || cmd.RegisterFlagCompletionFunc(name, fn) != nil {
			panic("completion for a flag that does not exist: " + path + " --" + name)
		}
	}
	flag("gateway get", "revision", positional(a, true, completeGatewayRevision))
	flag("gateway listener add", "protocol", cobra.FixedCompletions([]string{"tcp", "http", "https"}, cobra.ShellCompDirectiveNoFileComp))
}
