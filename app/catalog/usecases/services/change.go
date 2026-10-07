package services

import (
	"context"
	"strings"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/uuid"
)

func (u ServicesUsecase) Create(ctx context.Context, a actor.Actor, n entities.NewService) (entities.Service, error) {
	teamID, err := u.teams.IDBySlug(ctx, n.OwnerTeam)
	if err != nil {
		return entities.Service{}, err
	}
	if err := u.authz.Can(ctx, a, authz.CreateService, authz.Resource{TeamID: teamID}); err != nil {
		return entities.Service{}, err
	}
	s := newService(n, teamID)
	if err := s.Validate(); err != nil {
		return entities.Service{}, err
	}
	if err := u.store.Create(ctx, s); err != nil {
		return entities.Service{}, err
	}
	u.publisher.Publish(ctx, serviceEvent(a, "service.created", s, map[string]any{"name": s.Name}))
	return s, nil
}

func newService(n entities.NewService, teamID string) entities.Service {
	return entities.Service{
		ID: uuid.NewUUID(), Slug: n.Slug, Name: strings.TrimSpace(n.Name), Description: strings.TrimSpace(n.Description),
		OwnerTeamID: teamID, GitHubRepo: strings.TrimSpace(n.GitHubRepo), ImageRepository: strings.TrimSpace(n.ImageRepository),
		BuildWorkflow: entities.DefaultBuildWorkflow, CreatedAt: clock.Now().UTC(),
	}
}

func (u ServicesUsecase) Update(ctx context.Context, a actor.Actor, slug string, c entities.ServiceChanges) (entities.Service, error) {
	s, err := u.authorized(ctx, a, slug, authz.EditService)
	if err != nil {
		return entities.Service{}, err
	}
	updated := c.ApplyTo(s)
	if err := updated.Validate(); err != nil {
		return entities.Service{}, err
	}
	if err := u.store.Update(ctx, updated); err != nil {
		return entities.Service{}, err
	}
	u.publisher.Publish(ctx, serviceEvent(a, "service.updated", updated, map[string]any{"name": updated.Name}))
	return updated, nil
}

func (u ServicesUsecase) Transfer(ctx context.Context, a actor.Actor, slug, teamSlug string) (entities.Service, error) {
	s, err := u.authorized(ctx, a, slug, authz.TransferService)
	if err != nil {
		return entities.Service{}, err
	}
	teamID, err := u.teams.IDBySlug(ctx, teamSlug)
	if err != nil {
		return entities.Service{}, err
	}
	previous := s.OwnerTeamID
	s.OwnerTeamID = teamID
	if err := u.store.Update(ctx, s); err != nil {
		return entities.Service{}, err
	}
	u.publisher.Publish(ctx, serviceEvent(a, "service.transferred", s, map[string]any{"from_team_id": previous, "to_team": teamSlug}))
	return s, nil
}

func (u ServicesUsecase) Delete(ctx context.Context, a actor.Actor, slug string) error {
	s, err := u.authorized(ctx, a, slug, authz.DeleteService)
	if err != nil {
		return err
	}
	now := clock.Now().UTC()
	s.DeletedAt = &now
	if err := u.store.Update(ctx, s); err != nil {
		return err
	}
	u.publisher.Publish(ctx, serviceEvent(a, "service.deleted", s, nil))
	return nil
}

func (u ServicesUsecase) authorized(ctx context.Context, a actor.Actor, slug string, action authz.Action) (entities.Service, error) {
	s, err := u.store.FindBySlug(ctx, slug)
	if err != nil {
		return entities.Service{}, err
	}
	return s, u.authz.Can(ctx, a, action, authz.Resource{TeamID: s.OwnerTeamID})
}
