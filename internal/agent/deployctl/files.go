package deployctl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/adrianozp/platform/pkg/agentapi"
	"gopkg.in/yaml.v3"
)

type rollout struct {
	Steps []int  `yaml:"steps"`
	Bake  string `yaml:"bake"`
	Poll  string `yaml:"poll"`
}

type limits struct {
	MinRequests   int     `yaml:"min_requests"`
	MaxErrorRatio float64 `yaml:"max_error_ratio"`
	ErrorFactor   float64 `yaml:"error_factor"`
	MaxP95        string  `yaml:"max_p95"`
	LatencyFactor float64 `yaml:"latency_factor"`
	LatencySlack  string  `yaml:"latency_slack"`
}

type serviceFile struct {
	Name       string   `yaml:"name"`
	Kind       string   `yaml:"kind,omitempty"`
	Command    string   `yaml:"command,omitempty"`
	Port       int      `yaml:"port"`
	HealthPath string   `yaml:"health_path"`
	MemLimit   string   `yaml:"mem_limit"`
	Entrypoint string   `yaml:"entrypoint,omitempty"`
	Rule       string   `yaml:"rule,omitempty"`
	TLS        bool     `yaml:"tls,omitempty"`
	Rollout    *rollout `yaml:"rollout,omitempty"`
	Limits     *limits  `yaml:"limits,omitempty"`
}

var defaultLimits = limits{
	MinRequests: 30, MaxErrorRatio: 0.01, ErrorFactor: 2, MaxP95: "1s", LatencyFactor: 1.5, LatencySlack: "250ms",
}

func serviceDefinition(w agentapi.Workload) (serviceFile, error) {
	s := serviceFile{
		Name: w.Name, Port: w.Spec.Port, HealthPath: w.Spec.HealthPath, MemLimit: fmt.Sprintf("%dm", w.Spec.MemoryMB),
	}
	if w.Spec.Host == "" {
		return asWorker(s, w.Spec.Command)
	}
	s.Entrypoint, s.Rule, s.TLS = "websecure", "Host(`"+w.Spec.Host+"`)", true
	s.Rollout = &rollout{Steps: []int{10, 50, 100}, Bake: "1m", Poll: "15s"}
	s.Limits = &defaultLimits
	return s, nil
}

func asWorker(s serviceFile, command string) (serviceFile, error) {
	if command == "" {
		return serviceFile{}, fmt.Errorf("%s has no public host, so it runs as a worker and needs a command", s.Name)
	}
	s.Kind, s.Command = "worker", command
	return s, nil
}

func (r Runtime) writeFiles(w agentapi.Workload) error {
	def, err := serviceDefinition(w)
	if err != nil {
		return err
	}
	raw, err := yaml.Marshal(def)
	if err != nil {
		return err
	}
	if err := writeFile(filepath.Join(r.Root, "stack", "deploy", w.Name+".yml"), raw, 0o644); err != nil {
		return err
	}
	if err := writeFile(filepath.Join(r.Root, "config", w.Name+".env"), envFile(w.Env), 0o644); err != nil {
		return err
	}
	return writeFile(filepath.Join(r.Root, "secrets", w.Name+".env"), envFile(w.Secrets), 0o600)
}

func writeFile(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func envFile(values map[string]string) []byte {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(name + "=" + quote(values[name]) + "\n")
	}
	return []byte(b.String())
}

func quote(value string) string {
	if !strings.ContainsAny(value, "'\n\r") {
		return "'" + value + "'"
	}
	escaped := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "$", "$$").Replace(value)
	return `"` + escaped + `"`
}
