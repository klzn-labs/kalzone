package services

import (
	"time"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/internal/apispec"
)

type ServiceDTO struct {
	ID              string    `json:"id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	OwnerTeamID     string    `json:"owner_team_id"`
	GitHubRepo      string    `json:"github_repo"`
	ImageRepository string    `json:"image_repository"`
	AutoBuild       bool      `json:"auto_build"`
	AutoDeployDev   bool      `json:"auto_deploy_dev"`
	BuildWorkflow   string    `json:"build_workflow"`
	Status          string    `json:"status" enum:"active,decommissioning"`
	CreatedAt       time.Time `json:"created_at"`
}

func ToDTO(s entities.Service) ServiceDTO {
	return ServiceDTO{
		ID: s.ID, Slug: s.Slug, Name: s.Name, Description: s.Description, OwnerTeamID: s.OwnerTeamID,
		GitHubRepo: s.GitHubRepo, ImageRepository: s.ImageRepository, AutoBuild: s.AutoBuild, AutoDeployDev: s.AutoDeployDev,
		BuildWorkflow: s.BuildWorkflow, Status: string(s.Status), CreatedAt: s.CreatedAt,
	}
}

type createDTO struct {
	Slug            string `json:"slug" binding:"required"`
	Name            string `json:"name" binding:"required"`
	Description     string `json:"description,omitempty"`
	OwnerTeam       string `json:"owner_team" binding:"required"`
	GitHubRepo      string `json:"github_repo,omitempty"`
	ImageRepository string `json:"image_repository,omitempty"`
}

func (d createDTO) toEntity() entities.NewService {
	return entities.NewService{
		Slug: d.Slug, Name: d.Name, Description: d.Description, OwnerTeam: d.OwnerTeam,
		GitHubRepo: d.GitHubRepo, ImageRepository: d.ImageRepository,
	}
}

type updateDTO struct {
	Name            *string `json:"name,omitempty"`
	Description     *string `json:"description,omitempty"`
	GitHubRepo      *string `json:"github_repo,omitempty"`
	ImageRepository *string `json:"image_repository,omitempty"`
	AutoBuild       *bool   `json:"auto_build,omitempty"`
	AutoDeployDev   *bool   `json:"auto_deploy_dev,omitempty"`
	BuildWorkflow   *string `json:"build_workflow,omitempty"`
}

func (d updateDTO) toEntity() entities.ServiceChanges {
	return entities.ServiceChanges{
		Name: d.Name, Description: d.Description, GitHubRepo: d.GitHubRepo, ImageRepository: d.ImageRepository,
		AutoBuild: d.AutoBuild, AutoDeployDev: d.AutoDeployDev, BuildWorkflow: d.BuildWorkflow,
	}
}

type transferDTO struct {
	Team string `json:"team" binding:"required"`
}

type serviceParams struct {
	Service string `path:"service"`
}

type listParams struct {
	Team string `query:"team" description:"only this team's services"`
	apispec.ListParams
}

type teardownDTO struct {
	Environment  string `json:"environment"`
	Status       string `json:"status" enum:"removed,removing,awaiting_approval"`
	DeploymentID string `json:"deployment_id,omitempty"`
	RequestID    string `json:"request_id,omitempty"`
	ApproveURL   string `json:"approve_url,omitempty"`
}

type decommissionDTO struct {
	Service  ServiceDTO    `json:"service"`
	Deleted  bool          `json:"deleted"`
	Teardown []teardownDTO `json:"teardown"`
}

func toDecommissionDTO(d entities.Decommission) decommissionDTO {
	steps := make([]teardownDTO, 0, len(d.Steps))
	for _, s := range d.Steps {
		steps = append(steps, teardownDTO{
			Environment: s.Environment, Status: string(s.Status), DeploymentID: s.DeploymentID, RequestID: s.RequestID, ApproveURL: s.ApproveURL,
		})
	}
	return decommissionDTO{Service: ToDTO(d.Service), Deleted: d.Deleted, Teardown: steps}
}
