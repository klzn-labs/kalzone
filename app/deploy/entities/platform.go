package entities

import (
	"slices"
	"strings"
	"time"
)

const (
	PlatformControlPlane = "controlplane"
	OutcomeDeployed      = "deployed"
	OutcomeRolledBack    = "rolled_back"
)

var PlatformComponents = []string{PlatformControlPlane}

func IsPlatformComponent(name string) bool {
	return slices.Contains(PlatformComponents, name)
}

type PlatformRelease struct {
	Version   string    `json:"version"`
	Image     string    `json:"image"`
	Colour    string    `json:"colour"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (r PlatformRelease) Commit() string {
	return commitOf(r.Version)
}

func (r PlatformRelease) Config() string {
	_, config, _ := strings.Cut(r.Version, "+")
	return config
}

func commitOf(version string) string {
	commit, _, _ := strings.Cut(version, "+")
	return commit
}

type PlatformContainer struct {
	Found         bool      `json:"found"`
	Name          string    `json:"name"`
	State         string    `json:"state"`
	Health        string    `json:"health"`
	Restarts      int       `json:"restarts"`
	StartedAt     time.Time `json:"started_at"`
	MemoryLimitMB int       `json:"memory_limit_mb"`
}

type PlatformDeploy struct {
	At       time.Time `json:"at"`
	Version  string    `json:"version"`
	Outcome  string    `json:"outcome"`
	Step     string    `json:"step,omitempty"`
	Reason   string    `json:"reason,omitempty"`
	Strategy string    `json:"strategy"`
	Colour   string    `json:"colour"`
	RunURL   string    `json:"run_url,omitempty"`
}

func (d PlatformDeploy) Commit() string {
	return commitOf(d.Version)
}

func (d PlatformDeploy) RolledBack() bool {
	return d.Outcome == OutcomeRolledBack
}

type PlatformComponent struct {
	Name       string
	Live       PlatformRelease
	Container  PlatformContainer
	History    []PlatformDeploy
	Agent      string
	ObservedAt time.Time
}

func (c PlatformComponent) Healthy() bool {
	return c.Container.Found && c.Container.State == "running" && c.Container.Health != "unhealthy"
}

func (c PlatformComponent) Stale(now time.Time) bool {
	return now.Sub(c.ObservedAt) > StaleAfter
}

func (c PlatformComponent) LastRollback() (PlatformDeploy, bool) {
	for _, d := range c.History {
		if d.RolledBack() {
			return d, true
		}
	}
	return PlatformDeploy{}, false
}
