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

type runtimeStatus struct {
	ServiceID      string `gorm:"primaryKey"`
	Environment    string `gorm:"primaryKey"`
	Container      string
	Found          bool
	State          string
	Health         string
	HealthLog      []byte
	ExitCode       int
	OOMKilled      bool `gorm:"column:oom_killed"`
	Restarts       int
	Image          string
	MemoryLimitMB  int `gorm:"column:memory_limit_mb"`
	CPULimitMillis int `gorm:"column:cpu_limit_millis"`
	StartedAt      *time.Time
	ObservedAt     time.Time
	Agent          string
}

func (runtimeStatus) TableName() string { return "runtime_statuses" }

func fromRuntime(s entities.RuntimeStatus) runtimeStatus {
	healthLog, _ := json.Marshal(nonNil(s.HealthLog))
	return runtimeStatus{
		ServiceID: s.ServiceID, Environment: s.Environment, Container: s.Container, Found: s.Found, State: s.State,
		Health: s.Health, HealthLog: healthLog, ExitCode: s.ExitCode, OOMKilled: s.OOMKilled, Restarts: s.Restarts,
		Image: s.Image, MemoryLimitMB: s.MemoryLimitMB, CPULimitMillis: s.CPULimitMillis, StartedAt: s.StartedAt,
		ObservedAt: s.ObservedAt, Agent: s.Agent,
	}
}

func (m runtimeStatus) toEntity() entities.RuntimeStatus {
	var healthLog []string
	_ = json.Unmarshal(m.HealthLog, &healthLog)
	return entities.RuntimeStatus{
		ServiceID: m.ServiceID, Environment: m.Environment, Container: m.Container, Found: m.Found, State: m.State,
		Health: m.Health, HealthLog: healthLog, ExitCode: m.ExitCode, OOMKilled: m.OOMKilled, Restarts: m.Restarts,
		Image: m.Image, MemoryLimitMB: m.MemoryLimitMB, CPULimitMillis: m.CPULimitMillis, StartedAt: m.StartedAt,
		ObservedAt: m.ObservedAt, Agent: m.Agent,
	}
}

func nonNil(lines []string) []string {
	if lines == nil {
		return []string{}
	}
	return lines
}

func (r DeploymentRepository) SaveRuntime(ctx context.Context, statuses []entities.RuntimeStatus) error {
	if len(statuses) == 0 {
		return nil
	}
	models := make([]runtimeStatus, 0, len(statuses))
	for _, s := range statuses {
		models = append(models, fromRuntime(s))
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&models).Error
}

func (r DeploymentRepository) DeleteRuntime(ctx context.Context, serviceID string) error {
	return r.db.WithContext(ctx).Where("service_id = ?", serviceID).Delete(&runtimeStatus{}).Error
}

func (r DeploymentRepository) Runtime(ctx context.Context, serviceID, environment string) (entities.RuntimeStatus, bool, error) {
	var model runtimeStatus
	err := r.db.WithContext(ctx).Where("service_id = ? AND environment = ?", serviceID, environment).Take(&model).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return entities.RuntimeStatus{}, false, nil
	}
	return model.toEntity(), err == nil, err
}
