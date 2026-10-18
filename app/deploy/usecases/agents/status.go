package agents

import (
	"context"
	"time"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

func (u AgentsUsecase) ReportStatus(ctx context.Context, agent string, at time.Time, snapshots []entities.Snapshot) error {
	statuses := make([]entities.RuntimeStatus, 0, len(snapshots))
	for _, snapshot := range snapshots {
		service, err := u.services.FindService(ctx, snapshot.Service)
		if apperror.CodeOf(err) == apperror.CodeNotFound {
			continue
		}
		if err != nil {
			return err
		}
		statuses = append(statuses, stamped(snapshot, service.ID, agent, at))
	}
	return u.store.SaveRuntime(ctx, statuses)
}

func stamped(snapshot entities.Snapshot, serviceID, agent string, at time.Time) entities.RuntimeStatus {
	status := snapshot.Status
	status.ServiceID, status.Environment, status.Agent, status.ObservedAt = serviceID, snapshot.Environment, agent, at
	status.HealthLog = entities.Failure{HealthLog: status.HealthLog}.Clipped().HealthLog
	return status
}
