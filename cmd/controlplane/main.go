package main

import (
	"github.com/adrianozp/platform/cmd/controlplane/options"
	"github.com/adrianozp/platform/internal/httpserver"
	"go.uber.org/fx"
)

func main() {
	fx.New(options.Options(), fx.Invoke(httpserver.Run)).Run()
}
