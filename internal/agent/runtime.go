package agent

import (
	"context"
	"io"

	"github.com/adrianozp/platform/pkg/agentapi"
)

type Observed struct {
	Known   bool
	Running bool
	Image   string
}

type Runtime interface {
	Apply(ctx context.Context, w agentapi.Workload, out io.Writer) error
	Observe(ctx context.Context, name string) (Observed, error)
}
