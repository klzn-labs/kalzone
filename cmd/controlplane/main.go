package main

import (
	"github.com/klzn-labs/kalzone/cmd/controlplane/options"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"go.uber.org/fx"
)

func main() {
	fx.New(options.Options(), fx.Invoke(httpserver.Run)).Run()
}
