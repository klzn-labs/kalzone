package entities

import "time"

type Environment struct {
	Slug      string
	Name      string
	Protected bool
	Position  int
}

type Service struct {
	ID              string
	Slug            string
	Name            string
	Description     string
	OwnerTeamID     string
	GitHubRepo      string
	ImageRepository string
	AutoBuild       bool
	AutoDeployDev   bool
	BuildWorkflow   string
	Status          Status
	Limits          Limits
	CreatedAt       time.Time
	DeletedAt       *time.Time
}

type Limits struct {
	ResponseMS   int
	CPUPercent   int
	DBCPUPercent int
}

type Status string

const (
	StatusActive          Status = "active"
	StatusDecommissioning Status = "decommissioning"
)

func (s Service) Decommissioning() bool {
	return s.Status == StatusDecommissioning
}

type ServiceChanges struct {
	Name              *string
	Description       *string
	GitHubRepo        *string
	ImageRepository   *string
	AutoBuild         *bool
	AutoDeployDev     *bool
	BuildWorkflow     *string
	ResponseLimitMS   *int
	CPULimitPercent   *int
	DBCPULimitPercent *int
}

type ServiceFilter struct {
	OwnerTeamIDs []string
	GitHubRepo   string
}

const DefaultBuildWorkflow = "platform-build.yml"

type NewService struct {
	Slug            string
	Name            string
	Description     string
	OwnerTeam       string
	GitHubRepo      string
	ImageRepository string
}
