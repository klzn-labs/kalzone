package configs

import (
	"sort"
	"time"

	"github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/internal/apispec"
)

type specDTO struct {
	Port       int      `json:"port"`
	HealthPath string   `json:"health_path"`
	Host       string   `json:"host"`
	Aliases    []string `json:"aliases,omitempty"`
	Internal   bool     `json:"internal,omitempty"`
	MemoryMB   int      `json:"memory_mb"`
	CPUMillis  int      `json:"cpu_millis"`
	Replicas   int      `json:"replicas"`
	Command    string   `json:"command,omitempty"`
}

type revisionDTO struct {
	Number    int               `json:"number"`
	Spec      specDTO           `json:"spec"`
	Env       map[string]string `json:"env"`
	Secrets   []string          `json:"secrets"`
	AuthorID  string            `json:"author_id"`
	Message   string            `json:"message"`
	CreatedAt *time.Time        `json:"created_at"`
}

func toDTO(r entities.Revision) revisionDTO {
	dto := revisionDTO{
		Number: r.Number, Spec: specDTO(r.Spec), Env: r.Env, Secrets: secretNames(r.Secrets),
		AuthorID: r.AuthorID, Message: r.Message,
	}
	if !r.CreatedAt.IsZero() {
		dto.CreatedAt = &r.CreatedAt
	}
	return dto
}

func secretNames(secrets map[string]string) []string {
	names := make([]string, 0, len(secrets))
	for name := range secrets {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

type changeDTO struct {
	Spec     *specDTO          `json:"spec,omitempty" description:"replaces the whole spec"`
	SetEnv   map[string]string `json:"set_env,omitempty"`
	UnsetEnv []string          `json:"unset_env,omitempty"`
	Message  string            `json:"message,omitempty"`
}

func (d changeDTO) toEntity() entities.ChangeSet {
	cs := entities.ChangeSet{SetEnv: d.SetEnv, UnsetEnv: d.UnsetEnv, Message: d.Message}
	if d.Spec != nil {
		spec := entities.Spec(*d.Spec)
		cs.Spec = &spec
	}
	return cs
}

type patchDTO struct {
	Spec    map[string]any     `json:"spec,omitempty" description:"spec fields to change, merged into the current spec; null removes an optional field"`
	Env     map[string]*string `json:"env,omitempty" description:"variables to set; null removes one"`
	Message string             `json:"message,omitempty"`
}

type secretDTO struct {
	Value string `json:"value" binding:"required"`
}

type secretInfoDTO struct {
	Name  string    `json:"name"`
	SetBy string    `json:"set_by"`
	SetAt time.Time `json:"set_at"`
}

func toSecretDTO(i entities.SecretInfo) secretInfoDTO {
	return secretInfoDTO{Name: i.Name, SetBy: i.SetBy, SetAt: i.SetAt}
}

func secretKey(i entities.SecretInfo) string {
	return i.Name
}

type configParams struct {
	Service     string `path:"service"`
	Environment string `path:"env"`
}

type pagedParams struct {
	Service     string `path:"service"`
	Environment string `path:"env"`
	apispec.ListParams
}

type secretParams struct {
	Service     string `path:"service"`
	Environment string `path:"env"`
	Name        string `path:"name"`
}
