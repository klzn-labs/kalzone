package databases

import (
	"time"

	"github.com/klzn-labs/kalzone/app/database/entities"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
)

const nextDeployNote = "takes effect on each service's next deploy"

type AttachmentDTO struct {
	Service    string     `json:"service"`
	AttachedAt time.Time  `json:"attached_at"`
	DetachedAt *time.Time `json:"detached_at,omitempty"`
	Draining   bool       `json:"draining"`
}

type DatabaseDTO struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	Environment    string          `json:"environment"`
	TeamID         string          `json:"team_id"`
	Host           string          `json:"host"`
	Port           int             `json:"port"`
	Schema         string          `json:"schema"`
	Username       string          `json:"username"`
	MaxConnections int             `json:"max_connections"`
	Adopted        bool            `json:"adopted"`
	Attachments    []AttachmentDTO `json:"attachments"`
	Note           string          `json:"note,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

type createDTO struct {
	Name           string `json:"name" binding:"required"`
	Environment    string `json:"environment" binding:"required"`
	Team           string `json:"team" binding:"required"`
	MaxConnections int    `json:"max_connections,omitempty" description:"most connections the database user may hold at once, 1 to 30; 10 when left out"`
}

type attachDTO struct {
	Service string `json:"service" binding:"required"`
}

type described struct {
	database    entities.Database
	attachments []databases.AttachmentView
}

func toDTO(d described, endpoint databases.Endpoint) DatabaseDTO {
	db := d.database
	return DatabaseDTO{
		ID: db.ID, Name: db.Name, Environment: db.Environment, TeamID: db.TeamID, Host: endpoint.Host, Port: endpoint.Port,
		Schema: db.Schema, Username: db.Username, MaxConnections: db.MaxConnections, Adopted: db.Adopted,
		Attachments: toAttachments(d.attachments), CreatedAt: db.CreatedAt, UpdatedAt: db.UpdatedAt,
	}
}

func toAttachments(views []databases.AttachmentView) []AttachmentDTO {
	result := make([]AttachmentDTO, 0, len(views))
	for _, v := range views {
		result = append(result, AttachmentDTO{Service: v.Service, AttachedAt: v.AttachedAt, DetachedAt: v.DetachedAt, Draining: v.Draining})
	}
	return result
}

func describedKey(d described) string {
	return d.database.Environment + "/" + d.database.Name
}
