package services

import (
	"time"

	"github.com/adrianozp/platform/app/catalog/entities"
)

type ServiceDTO struct {
	ID              string    `json:"id"`
	Slug            string    `json:"slug"`
	Name            string    `json:"name"`
	Description     string    `json:"description"`
	OwnerTeamID     string    `json:"owner_team_id"`
	GitHubRepo      string    `json:"github_repo"`
	ImageRepository string    `json:"image_repository"`
	AutoBuild       bool      `json:"auto_build"`
	BuildWorkflow   string    `json:"build_workflow"`
	CreatedAt       time.Time `json:"created_at"`
}

func ToDTO(s entities.Service) ServiceDTO {
	return ServiceDTO{
		ID: s.ID, Slug: s.Slug, Name: s.Name, Description: s.Description, OwnerTeamID: s.OwnerTeamID,
		GitHubRepo: s.GitHubRepo, ImageRepository: s.ImageRepository, AutoBuild: s.AutoBuild, BuildWorkflow: s.BuildWorkflow,
		CreatedAt: s.CreatedAt,
	}
}

type createDTO struct {
	Slug            string `json:"slug" binding:"required"`
	Name            string `json:"name" binding:"required"`
	Description     string `json:"description"`
	OwnerTeam       string `json:"owner_team" binding:"required"`
	GitHubRepo      string `json:"github_repo"`
	ImageRepository string `json:"image_repository"`
}

func (d createDTO) toEntity() entities.NewService {
	return entities.NewService{
		Slug: d.Slug, Name: d.Name, Description: d.Description, OwnerTeam: d.OwnerTeam,
		GitHubRepo: d.GitHubRepo, ImageRepository: d.ImageRepository,
	}
}

type updateDTO struct {
	Name            *string `json:"name"`
	Description     *string `json:"description"`
	GitHubRepo      *string `json:"github_repo"`
	ImageRepository *string `json:"image_repository"`
	AutoBuild       *bool   `json:"auto_build"`
	BuildWorkflow   *string `json:"build_workflow"`
}

func (d updateDTO) toEntity() entities.ServiceChanges {
	return entities.ServiceChanges{
		Name: d.Name, Description: d.Description, GitHubRepo: d.GitHubRepo, ImageRepository: d.ImageRepository,
		AutoBuild: d.AutoBuild, BuildWorkflow: d.BuildWorkflow,
	}
}

type transferDTO struct {
	Team string `json:"team" binding:"required"`
}
