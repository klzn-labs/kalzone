package deployctl_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/adrianozp/platform/internal/agent/deployctl"
	"github.com/adrianozp/platform/pkg/agentapi"
	"github.com/stretchr/testify/require"
)

const fakeDeployctl = `#!/bin/sh
echo "$@" > "$(dirname "$0")/args"
echo "switching traffic"
exit "${FAKE_EXIT:-0}"
`

const fakeDocker = `#!/bin/sh
[ "$4" = "api-dev-blue" ] && echo true && exit 0
exit 1
`

func setup(t *testing.T) (deployctl.Runtime, string) {
	t.Helper()
	root, bin := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "deployctl"), []byte(fakeDeployctl), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), []byte(fakeDocker), 0o755))
	runtime := deployctl.Runtime{
		Root: root, Deployctl: filepath.Join(bin, "deployctl"), Docker: filepath.Join(bin, "docker"),
		RunURL:              func(w agentapi.Workload) string { return "https://platform/" + w.Name },
		InternalEntrypoints: map[string]string{"dev": "backend-dev"},
	}
	return runtime, bin
}

func workload(host, command string) agentapi.Workload {
	return agentapi.Workload{
		DeploymentID: "d1", Name: "api-dev", Service: "api", Environment: "dev", ImageRef: "docker.io/o/api@sha256:1",
		Version: "r3.c2", Strategy: "bluegreen",
		Spec:    agentapi.Spec{Port: 8080, HealthPath: "/healthz", Host: host, MemoryMB: 256, Command: command},
		Env:     map[string]string{"LOG_LEVEL": "debug", "QUOTE": "it's"},
		Secrets: map[string]string{"DB_PASSWORD": "p@ss$word"},
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(raw)
}

func Test_Apply_WritesTheFilesAndRunsDeployctl(t *testing.T) {
	runtime, bin := setup(t)
	var out bytes.Buffer

	require.NoError(t, runtime.Apply(context.Background(), workload("api.dev.kalz.one", ""), &out))

	service := read(t, filepath.Join(runtime.Root, "stack", "deploy", "api-dev.yml"))
	require.Contains(t, service, "rule: Host(`api.dev.kalz.one`)")
	require.Contains(t, service, "mem_limit: 256m")
	require.Contains(t, service, "entrypoint: websecure")
	require.Equal(t, "LOG_LEVEL='debug'\nQUOTE=\"it's\"\n", read(t, filepath.Join(runtime.Root, "config", "api-dev.env")))
	require.Equal(t, "DB_PASSWORD='p@ss$word'\n", read(t, filepath.Join(runtime.Root, "secrets", "api-dev.env")))
	info, err := os.Stat(filepath.Join(runtime.Root, "secrets", "api-dev.env"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	args := strings.TrimSpace(read(t, filepath.Join(bin, "args")))
	require.Equal(t, "deploy -root "+runtime.Root+" -service api-dev -image docker.io/o/api@sha256:1 -version r3.c2 -strategy bluegreen -run-url https://platform/api-dev", args)
	require.Equal(t, "switching traffic\n", out.String())
}

func Test_Apply_AServiceWithoutHostIsAWorker(t *testing.T) {
	runtime, _ := setup(t)

	require.NoError(t, runtime.Apply(context.Background(), workload("", "./worker"), &bytes.Buffer{}))
	err := runtime.Apply(context.Background(), workload("", ""), &bytes.Buffer{})

	service := read(t, filepath.Join(runtime.Root, "stack", "deploy", "api-dev.yml"))
	require.Contains(t, service, "kind: worker")
	require.Contains(t, service, "command: ./worker")
	require.NotContains(t, service, "rule:")
	require.ErrorContains(t, err, "needs a command")
}

func Test_Apply_RoutesEveryAlias(t *testing.T) {
	runtime, _ := setup(t)
	w := workload("kalz.one", "")
	w.Spec.Aliases = []string{"www.kalz.one"}

	require.NoError(t, runtime.Apply(context.Background(), w, &bytes.Buffer{}))

	service := read(t, filepath.Join(runtime.Root, "stack", "deploy", "api-dev.yml"))
	require.Contains(t, service, "rule: Host(`kalz.one`) || Host(`www.kalz.one`)")
}

func Test_Apply_AnInternalServiceTakesItsEnvironmentsEntrypoint(t *testing.T) {
	runtime, _ := setup(t)
	w := workload("", "")
	w.Spec.Internal = true
	unmapped := w
	unmapped.Environment = "prod"

	require.NoError(t, runtime.Apply(context.Background(), w, &bytes.Buffer{}))
	err := runtime.Apply(context.Background(), unmapped, &bytes.Buffer{})

	service := read(t, filepath.Join(runtime.Root, "stack", "deploy", "api-dev.yml"))
	require.Contains(t, service, "entrypoint: backend-dev")
	require.Contains(t, service, "rule: PathPrefix(`/`)")
	require.Contains(t, service, "rollout:")
	require.NotContains(t, service, "tls:")
	require.ErrorContains(t, err, "no internal entrypoint for prod")
}

func Test_Apply_ReportsAFailedRollout(t *testing.T) {
	runtime, _ := setup(t)
	t.Setenv("FAKE_EXIT", "1")

	err := runtime.Apply(context.Background(), workload("api.dev.kalz.one", ""), &bytes.Buffer{})

	require.ErrorContains(t, err, "deployctl")
}

func Test_Observe(t *testing.T) {
	runtime, _ := setup(t)
	never, err := runtime.Observe(context.Background(), "api-dev")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(runtime.Root, "state"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(runtime.Root, "state", "api-dev.json"), []byte(`{"colour":"blue","image":"img@sha256:1"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(runtime.Root, "state", "api-prod.json"), []byte(`{"colour":"green","image":"img@sha256:1"}`), 0o644))

	live, _ := runtime.Observe(context.Background(), "api-dev")
	stopped, _ := runtime.Observe(context.Background(), "api-prod")

	require.False(t, never.Known)
	require.True(t, live.Running)
	require.Equal(t, "img@sha256:1", live.Image)
	require.True(t, stopped.Known)
	require.False(t, stopped.Running)
}
