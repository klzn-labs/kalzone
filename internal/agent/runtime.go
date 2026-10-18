package agent

import (
	"context"
	"io"

	"github.com/klzn-labs/kalzone/pkg/agentapi"
)

type Observed struct {
	Known     bool
	Running   bool
	Image     string
	Container agentapi.Container
}

type Runtime interface {
	Apply(ctx context.Context, w agentapi.Workload, out io.Writer) error
	Observe(ctx context.Context, name string) (Observed, error)
	Inspect(ctx context.Context, container string) (agentapi.Container, error)
	Logs(ctx context.Context, container string, lines int) ([]string, error)
}
