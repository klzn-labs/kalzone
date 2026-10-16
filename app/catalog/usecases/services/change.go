package services

import (
	"context"
	"strings"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
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
		BuildWorkflow: entities.DefaultBuildWorkflow, Status: entities.StatusActive, CreatedAt: clock.Now().UTC(),
	}
}

func (u ServicesUsecase) Update(ctx context.Context, a actor.Actor, slug string, c entities.ServiceChanges) (entities.Service, error) {
	s, err := u.changeable(ctx, a, slug, authz.EditService)
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
	s, teamID, err := u.transferable(ctx, a, slug, teamSlug)
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

func (u ServicesUsecase) transferable(ctx context.Context, a actor.Actor, slug, teamSlug string) (entities.Service, string, error) {
	s, err := u.store.FindBySlug(ctx, slug)
	if err != nil {
		return entities.Service{}, "", err
	}
	if s.Decommissioning() {
		return entities.Service{}, "", errDecommissioning
	}
	teamID, err := u.teams.IDBySlug(ctx, teamSlug)
	if err != nil {
		return entities.Service{}, "", err
	}
	resource := authz.Resource{TeamID: s.OwnerTeamID, DestinationTeamID: teamID}
	return s, teamID, u.authz.Can(ctx, a, authz.TransferService, resource)
}

func (u ServicesUsecase) MarkDecommissioning(ctx context.Context, a actor.Actor, slug string) (entities.Service, error) {
	s, err := u.authorized(ctx, a, slug, authz.DeleteService)
	if err != nil || s.Decommissioning() {
		return s, err
	}
	s.Status = entities.StatusDecommissioning
	if err := u.store.Update(ctx, s); err != nil {
		return entities.Service{}, err
	}
	u.publisher.Publish(ctx, serviceEvent(a, "service.decommissioning", s, nil))
	return s, nil
}

func (u ServicesUsecase) MarkDeleted(ctx context.Context, a actor.Actor, id string) error {
	s, err := u.store.FindByID(ctx, id)
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

var errDecommissioning = apperror.Conflict("this service is being deleted").WithHint("it cannot change until the teardown finishes")

func (u ServicesUsecase) changeable(ctx context.Context, a actor.Actor, slug string, action authz.Action) (entities.Service, error) {
	s, err := u.authorized(ctx, a, slug, action)
	if err == nil && s.Decommissioning() {
		return entities.Service{}, errDecommissioning
	}
	return s, err
}

func (u ServicesUsecase) authorized(ctx context.Context, a actor.Actor, slug string, action authz.Action) (entities.Service, error) {
	s, err := u.store.FindBySlug(ctx, slug)
	if err != nil {
		return entities.Service{}, err
	}
	return s, u.authz.Can(ctx, a, action, authz.Resource{TeamID: s.OwnerTeamID})
}
