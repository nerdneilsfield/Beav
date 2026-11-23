// Package main is the entry point for the beav CLI application.
// Package main 是 beav CLI 应用程序的入口点。
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/dengqi/beav/internal/cli"
	"github.com/dengqi/beav/internal/i18n"
)

// Build-time version information set via ldflags.
// 构建时版本信息，通过 ldflags 设置。
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	lang := i18n.DetectFromArgs(os.Args[1:], os.Getenv)
	tr := i18n.New(lang)
	root := cli.NewRootCmdWithLang(version, commit, date, lang)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := root.ExecuteContext(ctx)
	stop()
	if err != nil {
		var cliErr cli.ExitError
		if errors.As(err, &cliErr) {
			fmt.Fprint(os.Stderr, tr.Textf("Error: %s\n", "错误：%s\n", cliErr.Error()))
			os.Exit(cliErr.Code())
		}
		if errors.Is(err, context.Canceled) {
			fmt.Fprint(os.Stderr, tr.Textf("Canceled: %s\n", "已取消：%s\n", err.Error()))
			os.Exit(4)
		}
		fmt.Fprint(os.Stderr, tr.Textf("Error: %s\n", "错误：%s\n", err.Error()))
		fmt.Fprintln(os.Stderr, tr.Text("Run `beav --help` for usage.", "运行 `beav --help` 查看用法。"))
		os.Exit(1)
	}
}
