package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/hasankaradmir/karga-cli/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	go func() {
		<-ctx.Done()
		stop() // a second Ctrl-C ends karga at once
	}()
	err := cli.NewRootCmd(os.Stdout, os.Stderr).ExecuteContext(ctx)
	stop()
	var exit *cli.ExitCodeError
	if err != nil && !errors.As(err, &exit) { // with its own exit code, a command printed what it had to say
		fmt.Fprintln(os.Stderr, "Error:", cli.Printable(err.Error()))
		if hint := cli.Hint(err); hint != "" {
			fmt.Fprintln(os.Stderr, "Hint:", hint)
		}
	}
	os.Exit(cli.ExitCode(err))
}
