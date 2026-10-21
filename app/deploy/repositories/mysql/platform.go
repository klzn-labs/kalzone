package mysql

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type platformComponent struct {
	Name       string `gorm:"primaryKey"`
	Payload    []byte
	Agent      string
	ObservedAt time.Time
}

func (platformComponent) TableName() string { return "platform_components" }

type platformPayload struct {
	Live      entities.PlatformRelease   `json:"live"`
	Container entities.PlatformContainer `json:"container"`
	History   []entities.PlatformDeploy  `json:"history"`
}

func (r DeploymentRepository) SavePlatform(ctx context.Context, c entities.PlatformComponent) error {
	payload, err := json.Marshal(platformPayload{Live: c.Live, Container: c.Container, History: c.History})
	if err != nil {
		return err
	}
	model := platformComponent{Name: c.Name, Payload: payload, Agent: c.Agent, ObservedAt: c.ObservedAt}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&model).Error
}

func (r DeploymentRepository) Platform(ctx context.Context, name string) (entities.PlatformComponent, bool, error) {
	var model platformComponent
	err := r.db.WithContext(ctx).Where("name = ?", name).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.PlatformComponent{}, false, nil
	}
	if err != nil {
		return entities.PlatformComponent{}, false, err
	}
	var payload platformPayload
	err = json.Unmarshal(model.Payload, &payload)
	return entities.PlatformComponent{
		Name: model.Name, Live: payload.Live, Container: payload.Container, History: payload.History,
		Agent: model.Agent, ObservedAt: model.ObservedAt,
	}, err == nil, err
}
