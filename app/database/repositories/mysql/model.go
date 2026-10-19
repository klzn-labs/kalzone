package mysql

import (
	"time"

	"github.com/klzn-labs/kalzone/app/database/entities"
)

type databaseModel struct {
	ID                 string `gorm:"primaryKey"`
	Name               string
	Environment        string
	TeamID             string
	SchemaName         string
	Username           string
	PasswordCiphertext []byte
	PasswordNonce      []byte
	PasswordKeyID      string
	MaxConnections     int
	Adopted            bool
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (databaseModel) TableName() string { return "databases" }

type attachmentModel struct {
	DatabaseID  string `gorm:"primaryKey"`
	ServiceID   string `gorm:"primaryKey"`
	Environment string
	AttachedAt  time.Time
	DetachedAt  *time.Time
}

func (attachmentModel) TableName() string { return "database_attachments" }

func fromDatabase(d entities.Database) databaseModel {
	return databaseModel{
		ID: d.ID, Name: d.Name, Environment: d.Environment, TeamID: d.TeamID, SchemaName: d.Schema,
		Username: d.Username, PasswordCiphertext: d.PasswordCiphertext, PasswordNonce: d.PasswordNonce,
		PasswordKeyID: d.PasswordKeyID, MaxConnections: d.MaxConnections, Adopted: d.Adopted, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func (m databaseModel) toEntity() entities.Database {
	return entities.Database{
		ID: m.ID, Name: m.Name, Environment: m.Environment, TeamID: m.TeamID, Schema: m.SchemaName,
		Username: m.Username, PasswordCiphertext: m.PasswordCiphertext, PasswordNonce: m.PasswordNonce,
		PasswordKeyID: m.PasswordKeyID, MaxConnections: m.MaxConnections, Adopted: m.Adopted, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func fromAttachment(a entities.Attachment) attachmentModel {
	return attachmentModel(a)
}

func (m attachmentModel) toEntity() entities.Attachment {
	return entities.Attachment(m)
}
