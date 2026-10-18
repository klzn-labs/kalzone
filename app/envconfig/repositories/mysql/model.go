package mysql

import (
	"encoding/json"
	"time"

	"github.com/klzn-labs/kalzone/app/envconfig/entities"
)

type revision struct {
	ID          string `gorm:"primaryKey"`
	ServiceID   string
	Environment string
	Number      int
	Spec        []byte
	Env         []byte
	Secrets     []byte
	Host        string
	AuthorID    string
	Message     string
	CreatedAt   time.Time
}

func (revision) TableName() string { return "config_revisions" }

func fromRevision(r entities.Revision) (revision, error) {
	spec, specErr := json.Marshal(r.Spec)
	env, envErr := json.Marshal(r.Env)
	secrets, secretsErr := json.Marshal(r.Secrets)
	model := revision{
		ID: r.ID, ServiceID: r.ServiceID, Environment: r.Environment, Number: r.Number,
		Spec: spec, Env: env, Secrets: secrets, Host: r.Spec.Host,
		AuthorID: r.AuthorID, Message: r.Message, CreatedAt: r.CreatedAt,
	}
	return model, firstError(specErr, envErr, secretsErr)
}

func (m revision) toEntity() (entities.Revision, error) {
	r := entities.Revision{
		ID: m.ID, ServiceID: m.ServiceID, Environment: m.Environment, Number: m.Number,
		AuthorID: m.AuthorID, Message: m.Message, CreatedAt: m.CreatedAt,
	}
	specErr := json.Unmarshal(m.Spec, &r.Spec)
	envErr := json.Unmarshal(m.Env, &r.Env)
	secretsErr := json.Unmarshal(m.Secrets, &r.Secrets)
	return r, firstError(specErr, envErr, secretsErr)
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

type secretVersion struct {
	ID          string `gorm:"primaryKey"`
	ServiceID   string
	Environment string
	Name        string
	Ciphertext  []byte
	Nonce       []byte
	KeyID       string
	CreatedBy   string
	CreatedAt   time.Time
}

func (secretVersion) TableName() string { return "secret_versions" }

func fromSecretVersion(v entities.SecretVersion) secretVersion {
	return secretVersion{
		ID: v.ID, ServiceID: v.ServiceID, Environment: v.Environment, Name: v.Name,
		Ciphertext: v.Ciphertext, Nonce: v.Nonce, KeyID: v.KeyID, CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt,
	}
}

func (m secretVersion) toEntity() entities.SecretVersion {
	return entities.SecretVersion{
		ID: m.ID, ServiceID: m.ServiceID, Environment: m.Environment, Name: m.Name,
		Ciphertext: m.Ciphertext, Nonce: m.Nonce, KeyID: m.KeyID, CreatedBy: m.CreatedBy, CreatedAt: m.CreatedAt,
	}
}
