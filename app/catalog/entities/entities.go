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
	CreatedAt       time.Time
	DeletedAt       *time.Time
}

type ServiceChanges struct {
	Name            *string
	Description     *string
	GitHubRepo      *string
	ImageRepository *string
}

type ServiceFilter struct {
	OwnerTeamIDs []string
}

type NewService struct {
	Slug            string
	Name            string
	Description     string
	OwnerTeam       string
	GitHubRepo      string
	ImageRepository string
}
