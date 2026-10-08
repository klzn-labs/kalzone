package deployctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/adrianozp/platform/internal/agent"
	"github.com/adrianozp/platform/pkg/agentapi"
)

type Runtime struct {
	Root      string
	Deployctl string
	Docker    string
	RunURL    func(w agentapi.Workload) string

	InternalEntrypoints map[string]string
}

func New(cfg agent.Config) Runtime {
	base := strings.TrimRight(cfg.PlatformURL, "/")
	return Runtime{
		Root: cfg.Root, Deployctl: cfg.Deployctl, Docker: cfg.Docker, InternalEntrypoints: cfg.InternalEntrypoints,
		RunURL: func(w agentapi.Workload) string { return base + "/services/" + w.Service + "/env/" + w.Environment },
	}
}

func (r Runtime) Apply(ctx context.Context, w agentapi.Workload, out io.Writer) error {
	if err := r.writeFiles(w); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, r.Deployctl, "deploy",
		"-root", r.Root, "-service", w.Name, "-image", w.ImageRef, "-version", w.Version,
		"-strategy", w.Strategy, "-run-url", r.RunURL(w))
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("deployctl: %w", err)
	}
	return nil
}

type state struct {
	Colour string `json:"colour"`
	Image  string `json:"image"`
}

func (r Runtime) Observe(ctx context.Context, name string) (agent.Observed, error) {
	raw, err := os.ReadFile(filepath.Join(r.Root, "state", name+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return agent.Observed{}, nil
	}
	if err != nil {
		return agent.Observed{}, err
	}
	var live state
	if err := json.Unmarshal(raw, &live); err != nil {
		return agent.Observed{}, err
	}
	running, err := r.running(ctx, name+"-"+live.Colour)
	return agent.Observed{Known: true, Running: running, Image: live.Image}, err
}

func (r Runtime) running(ctx context.Context, container string) (bool, error) {
	out, err := exec.CommandContext(ctx, r.Docker, "inspect", "-f", "{{.State.Running}}", container).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "true", err
}
