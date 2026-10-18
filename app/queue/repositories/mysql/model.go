package mysql

import (
	"encoding/json"
	"time"

	"github.com/klzn-labs/kalzone/app/queue/entities"
)

type queueModel struct {
	ID          string `gorm:"primaryKey"`
	Name        string
	Environment string
	TeamID      string
	ConsumerID  string
	Version     int
	Paused      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (queueModel) TableName() string { return "queues" }

type versionModel struct {
	QueueID   string `gorm:"primaryKey"`
	Number    int    `gorm:"primaryKey"`
	Settings  []byte
	AuthorID  string
	Message   string
	CreatedAt time.Time
}

func (versionModel) TableName() string { return "queue_versions" }

type publisherModel struct {
	QueueID   string `gorm:"primaryKey"`
	ServiceID string `gorm:"primaryKey"`
}

func (publisherModel) TableName() string { return "queue_publishers" }

type credentialsModel struct {
	ServiceID             string `gorm:"primaryKey"`
	Environment           string `gorm:"primaryKey"`
	TokenCiphertext       []byte
	TokenNonce            []byte
	TokenHash             string
	KeyCiphertext         []byte
	KeyNonce              []byte
	PreviousKeyCiphertext []byte
	PreviousKeyNonce      []byte
	KeyID                 string
	RotatedAt             *time.Time
}

func (credentialsModel) TableName() string { return "queue_credentials" }

type commandModel struct {
	ID        string `gorm:"primaryKey"`
	QueueID   string
	Kind      string
	Argument  string
	CreatedAt time.Time
	DoneAt    *time.Time
}

func (commandModel) TableName() string { return "queue_commands" }

func fromQueue(q entities.Queue) queueModel {
	return queueModel{
		ID: q.ID, Name: q.Name, Environment: q.Environment, TeamID: q.TeamID, ConsumerID: q.ConsumerID,
		Version: q.Version, Paused: q.Paused, CreatedAt: q.CreatedAt, UpdatedAt: q.UpdatedAt,
	}
}

func fromVersion(v entities.Version) (versionModel, error) {
	raw, err := json.Marshal(v.Settings)
	return versionModel{
		QueueID: v.QueueID, Number: v.Number, Settings: raw, AuthorID: v.AuthorID,
		Message: v.Message, CreatedAt: v.CreatedAt,
	}, err
}

func (m versionModel) toEntity() (entities.Version, error) {
	v := entities.Version{QueueID: m.QueueID, Number: m.Number, AuthorID: m.AuthorID, Message: m.Message, CreatedAt: m.CreatedAt}
	return v, json.Unmarshal(m.Settings, &v.Settings)
}

func (m queueModel) toEntity(settings entities.Settings, publishers []string) entities.Queue {
	return entities.Queue{
		ID: m.ID, Name: m.Name, Environment: m.Environment, TeamID: m.TeamID, ConsumerID: m.ConsumerID,
		Version: m.Version, Settings: settings, Publishers: publishers, Paused: m.Paused,
		CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

func publisherModels(queueID string, services []string) []publisherModel {
	result := make([]publisherModel, 0, len(services))
	for _, id := range services {
		result = append(result, publisherModel{QueueID: queueID, ServiceID: id})
	}
	return result
}

func fromCredentials(c entities.Credentials) credentialsModel {
	return credentialsModel(c)
}

func (m credentialsModel) toEntity() entities.Credentials {
	return entities.Credentials(m)
}

func fromCommand(c entities.Command) commandModel {
	return commandModel(c)
}

func (m commandModel) toEntity() entities.Command {
	return entities.Command(m)
}
