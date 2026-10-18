package agents

import (
	"context"
	"strings"

	"github.com/klzn-labs/kalzone/app/deploy/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

func (u AgentsUsecase) Report(ctx context.Context, agent, deploymentID string, lines []string) error {
	d, err := u.running(ctx, agent, deploymentID)
	if err != nil {
		return err
	}
	return u.store.AppendEvents(ctx, d.ID, clock.Now().UTC(), clean(lines))
}

func (u AgentsUsecase) Finish(ctx context.Context, agent, deploymentID string, succeeded bool, message string, failure *entities.Failure) (entities.Deployment, error) {
	d, err := u.running(ctx, agent, deploymentID)
	if err != nil {
		return entities.Deployment{}, err
	}
	if failure != nil && !succeeded {
		clipped := failure.Clipped()
		d.Failure = &clipped
	}
	return u.finish(ctx, agent, d, succeeded, message)
}

func (u AgentsUsecase) finish(ctx context.Context, agent string, d entities.Deployment, succeeded bool, message string) (entities.Deployment, error) {
	now := clock.Now().UTC()
	d.Status, d.Message, d.FinishedAt = entities.StatusFailed, truncate(message, 500), &now
	action := "deploy.failed"
	if succeeded {
		d.Status, action = entities.StatusSucceeded, "deploy.succeeded"
	}
	if err := u.store.Transition(ctx, d, entities.StatusInProgress); err != nil {
		return entities.Deployment{}, err
	}
	u.publish(ctx, agentActor(agent), action, d, d.Message)
	return d, nil
}

func (u AgentsUsecase) running(ctx context.Context, agent, deploymentID string) (entities.Deployment, error) {
	d, err := u.store.FindByID(ctx, deploymentID)
	if err != nil {
		return entities.Deployment{}, err
	}
	if d.Status != entities.StatusInProgress {
		return entities.Deployment{}, errNotRunning
	}
	if d.Agent != agent {
		return entities.Deployment{}, errNotYours
	}
	return d, nil
}

func clean(lines []string) []string {
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		result = append(result, truncate(strings.TrimRight(line, "\r\n"), maxLineLength))
	}
	return result
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
