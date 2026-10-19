package resources

import (
	"time"

	"github.com/klzn-labs/kalzone/app/resources/entities"
)

type ResourceDTO struct {
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Environment string    `json:"environment"`
	TeamID      string    `json:"team_id"`
	Attached    []string  `json:"attached"`
	CreatedAt   time.Time `json:"created_at"`
}

func toDTO(r entities.Resource) ResourceDTO {
	attached := r.Attached
	if attached == nil {
		attached = []string{}
	}
	return ResourceDTO{
		Kind: string(r.Kind), Name: r.Name, Environment: r.Environment, TeamID: r.TeamID,
		Attached: attached, CreatedAt: r.CreatedAt,
	}
}

func resourceKey(r entities.Resource) string {
	return string(r.Kind) + "/" + r.Environment + "/" + r.Name
}
