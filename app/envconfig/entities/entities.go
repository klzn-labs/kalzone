package entities

import "time"

type Spec struct {
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

func DefaultSpec() Spec {
	return Spec{Port: 8080, HealthPath: "/healthz", MemoryMB: 256, CPUMillis: 250, Replicas: 1}
}

type Revision struct {
	ID          string
	ServiceID   string
	Environment string
	Number      int
	Spec        Spec
	Env         map[string]string
	Secrets     map[string]string
	AuthorID    string
	Message     string
	CreatedAt   time.Time
}

func Initial(serviceID, environment string) Revision {
	return Revision{
		ServiceID: serviceID, Environment: environment, Spec: DefaultSpec(),
		Env: map[string]string{}, Secrets: map[string]string{},
	}
}

func (r Revision) Next() Revision {
	next := r
	next.ID, next.AuthorID, next.Message = "", "", ""
	next.Number = r.Number + 1
	next.Env = copyMap(r.Env)
	next.Secrets = copyMap(r.Secrets)
	return next
}

func copyMap(m map[string]string) map[string]string {
	result := make(map[string]string, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

type SecretVersion struct {
	ID          string
	ServiceID   string
	Environment string
	Name        string
	Ciphertext  []byte
	Nonce       []byte
	KeyID       string
	CreatedBy   string
	CreatedAt   time.Time
}

type SecretInfo struct {
	Name      string
	VersionID string
	SetBy     string
	SetAt     time.Time
}

type ChangeSet struct {
	Spec     *Spec
	SetEnv   map[string]string
	UnsetEnv []string
	Message  string
}

type ServiceRef struct {
	ID          string
	Slug        string
	OwnerTeamID string
}

type Resolved struct {
	Revision Revision
	Secrets  map[string]string
}
