package entities

import "time"

type Status string

const (
	StatusRequested Status = "requested"
	StatusBuilding  Status = "building"
	StatusBuilt     Status = "built"
	StatusFailed    Status = "failed"
)

type Source string

const (
	SourceBuild      Source = "build"
	SourceRegistered Source = "registered"
	SourceShared     Source = "shared"
)

type Trigger string

const (
	TriggerManual   Trigger = "manual"
	TriggerPush     Trigger = "push"
	TriggerRegister Trigger = "register"
)

type Release struct {
	ID          string
	ServiceID   string
	Number      int
	Source      Source
	GitRef      string
	CommitSHA   string
	Image       string
	Tag         string
	Digest      string
	Status      Status
	Trigger     Trigger
	RequestedBy string
	RunURL      string
	Failure     string
	CreatedAt   time.Time
	FinishedAt  *time.Time
}

func (r Release) Final() bool {
	return r.Status == StatusBuilt || r.Status == StatusFailed
}

func (r Release) SharedWith(service ServiceRef, id string, at time.Time) Release {
	shared := r
	shared.ID, shared.ServiceID, shared.Number, shared.Source = id, service.ID, 0, SourceShared
	shared.CreatedAt, shared.FinishedAt = at, &at
	return shared
}

func (r Release) Reference() string {
	if r.Digest == "" {
		return ""
	}
	return r.Image + "@" + r.Digest
}

func (s ServiceRef) SharesImageWith(other ServiceRef) bool {
	return s.ID != other.ID && s.GitHubRepo == other.GitHubRepo && s.ImageRepository == other.ImageRepository
}

type ServiceRef struct {
	ID              string
	Slug            string
	OwnerTeamID     string
	GitHubRepo      string
	ImageRepository string
	BuildWorkflow   string
	AutoBuild       bool
	Decommissioning bool
	Simulated       bool
}
