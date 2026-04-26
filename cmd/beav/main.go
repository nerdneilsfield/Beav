package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dengqi/beav/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	root := cli.NewRootCmd(version, commit, date)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := root.ExecuteContext(ctx); err != nil {
		var cliErr cli.CLIError
		if errors.As(err, &cliErr) {
			fmt.Fprintln(os.Stderr, cliErr.Error())
			os.Exit(cliErr.Code())
		}
		if errors.Is(err, context.Canceled) {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(4)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
