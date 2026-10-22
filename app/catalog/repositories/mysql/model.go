package mysql

import (
	"time"

	"github.com/klzn-labs/kalzone/app/catalog/entities"
)

type environment struct {
	Slug      string `gorm:"primaryKey"`
	Name      string
	Protected bool
	Position  int
}

func (environment) TableName() string { return "environments" }

func (e environment) toEntity() entities.Environment {
	return entities.Environment{Slug: e.Slug, Name: e.Name, Protected: e.Protected, Position: e.Position}
}

type service struct {
	ID              string `gorm:"primaryKey"`
	Slug            string
	Name            string
	Description     string
	OwnerTeamID     string
	GitHubRepo      string `gorm:"column:github_repo"`
	ImageRepository string
	AutoBuild       bool
	AutoDeployDev   bool
	BuildWorkflow   string
	Status          string
	LimitResponseMS int `gorm:"column:limit_response_ms"`
	LimitCPU        int `gorm:"column:limit_cpu_percent"`
	LimitDBCPU      int `gorm:"column:limit_db_cpu_percent"`
	Simulated       bool
	CreatedAt       time.Time
	DeletedAt       *time.Time
}

func (service) TableName() string { return "services" }

func (s service) toEntity() entities.Service {
	return entities.Service{
		ID: s.ID, Slug: s.Slug, Name: s.Name, Description: s.Description, OwnerTeamID: s.OwnerTeamID,
		GitHubRepo: s.GitHubRepo, ImageRepository: s.ImageRepository, AutoBuild: s.AutoBuild, AutoDeployDev: s.AutoDeployDev,
		BuildWorkflow: s.BuildWorkflow, Status: entities.Status(s.Status),
		Limits:    entities.Limits{ResponseMS: s.LimitResponseMS, CPUPercent: s.LimitCPU, DBCPUPercent: s.LimitDBCPU},
		Simulated: s.Simulated, CreatedAt: s.CreatedAt, DeletedAt: s.DeletedAt,
	}
}

func fromService(s entities.Service) service {
	return service{
		ID: s.ID, Slug: s.Slug, Name: s.Name, Description: s.Description, OwnerTeamID: s.OwnerTeamID,
		GitHubRepo: s.GitHubRepo, ImageRepository: s.ImageRepository, AutoBuild: s.AutoBuild, AutoDeployDev: s.AutoDeployDev,
		BuildWorkflow: s.BuildWorkflow, Status: string(statusOrActive(s.Status)),
		LimitResponseMS: s.Limits.ResponseMS, LimitCPU: s.Limits.CPUPercent, LimitDBCPU: s.Limits.DBCPUPercent,
		Simulated: s.Simulated, CreatedAt: s.CreatedAt, DeletedAt: s.DeletedAt,
	}
}

func statusOrActive(s entities.Status) entities.Status {
	if s == "" {
		return entities.StatusActive
	}
	return s
}
