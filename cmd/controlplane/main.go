package main

import (
	"os"

	"github.com/klzn-labs/kalzone/cmd/controlplane/options"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"go.uber.org/fx"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == setupLinkCommand {
		os.Exit(runSetupLink(os.Args[2:], os.Stdout, os.Stderr))
	}
	fx.New(options.Options(), fx.Invoke(httpserver.Run)).Run()
}
