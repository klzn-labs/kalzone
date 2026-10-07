package mysql

import (
	"context"
	"time"

	"github.com/adrianozp/platform/app/deploy/entities"
	"gorm.io/gorm/clause"
)

func (r DeploymentRepository) AppendEvents(ctx context.Context, deploymentID string, at time.Time, lines []string) error {
	if len(lines) == 0 {
		return nil
	}
	models := make([]deploymentEvent, 0, len(lines))
	for _, line := range lines {
		models = append(models, deploymentEvent{DeploymentID: deploymentID, At: at, Message: line})
	}
	return r.db.WithContext(ctx).Create(&models).Error
}

func (r DeploymentRepository) Events(ctx context.Context, deploymentID string, limit int) ([]entities.Event, error) {
	var models []deploymentEvent
	err := r.db.WithContext(ctx).Where("deployment_id = ?", deploymentID).Order("id DESC").Limit(limit).Find(&models).Error
	if err != nil {
		return nil, err
	}
	result := make([]entities.Event, len(models))
	for i, m := range models {
		result[len(models)-1-i] = entities.Event{ID: m.ID, DeploymentID: m.DeploymentID, At: m.At, Message: m.Message}
	}
	return result, nil
}

func (r DeploymentRepository) TouchAgent(ctx context.Context, a entities.Agent) error {
	model := agent{Name: a.Name, Version: a.Version, LastSeenAt: a.LastSeenAt}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&model).Error
}

func (r DeploymentRepository) Agents(ctx context.Context) ([]entities.Agent, error) {
	var models []agent
	if err := r.db.WithContext(ctx).Order("name").Find(&models).Error; err != nil {
		return nil, err
	}
	result := make([]entities.Agent, 0, len(models))
	for _, m := range models {
		result = append(result, entities.Agent{Name: m.Name, Version: m.Version, LastSeenAt: m.LastSeenAt})
	}
	return result, nil
}
