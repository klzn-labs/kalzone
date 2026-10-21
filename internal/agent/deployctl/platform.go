package deployctl

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/klzn-labs/kalzone/pkg/agentapi"
)

func (r Runtime) Platform(ctx context.Context, name string) (agentapi.PlatformReport, bool, error) {
	var report agentapi.PlatformReport
	found, err := r.readState(name+".json", &report.Live)
	if !found || err != nil {
		return report, false, err
	}
	if _, err := r.readState(name+".history.json", &report.History); err != nil {
		return report, false, err
	}
	report.Container, err = r.Inspect(ctx, name+"-"+report.Live.Colour)
	return report, err == nil, err
}

func (r Runtime) readState(file string, into any) (bool, error) {
	raw, err := os.ReadFile(filepath.Join(r.Root, "state", file))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(raw, into)
}
