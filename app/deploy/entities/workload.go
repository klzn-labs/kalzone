package entities

import (
	"fmt"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

type ServiceRef struct {
	ID              string
	Slug            string
	OwnerTeamID     string
	AutoDeployDev   bool
	Decommissioning bool
}

type ReleaseRef struct {
	ID        string
	ServiceID string
	Number    int
	Built     bool
	Reference string
}

type RuntimeSpec struct {
	Port       int
	HealthPath string
	Host       string
	Aliases    []string
	Internal   bool
	MemoryMB   int
	CPUMillis  int
	Replicas   int
	Command    string
}

type ResolvedConfig struct {
	Number  int
	Spec    RuntimeSpec
	Env     map[string]string
	Secrets map[string]string
}

type Workload struct {
	Deployment Deployment
	Name       string
	Service    string
	Version    string
	Config     ResolvedConfig
}

type Desired struct {
	Name         string
	Service      string
	Environment  string
	ImageRef     string
	DeploymentID string
}

type Drift struct {
	Service     string
	Environment string
	Observed    string
}

func WorkloadName(serviceSlug, environment string) string {
	return serviceSlug + "-" + environment
}

func VersionLabel(d Deployment) string {
	return fmt.Sprintf("r%d.c%d", d.ReleaseNumber, d.ConfigRevision)
}

func ParseStrategy(raw string) (Strategy, error) {
	switch Strategy(raw) {
	case "", StrategyBlueGreen:
		return StrategyBlueGreen, nil
	case StrategyCanary:
		return StrategyCanary, nil
	}
	return "", apperror.Invalid("strategy must be bluegreen or canary")
}
