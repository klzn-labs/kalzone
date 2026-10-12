package members

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/events"
)

type target struct {
	team    entities.Team
	user    entities.UserRef
	current entities.Membership
	exists  bool
}

func (u MembersUsecase) Put(ctx context.Context, a actor.Actor, teamSlug, login string, role entities.Role) (entities.Member, error) {
	if !role.Valid() {
		return entities.Member{}, errInvalidRole
	}
	t, err := u.resolve(ctx, a, teamSlug, login)
	if err != nil {
		return entities.Member{}, err
	}
	if err := u.guardLastMaintainer(ctx, a, t, role); err != nil {
		return entities.Member{}, err
	}
	membership := entities.Membership{TeamID: t.team.ID, UserID: t.user.ID, Role: role, CreatedAt: clock.Now().UTC()}
	if err := u.store.PutMembership(ctx, membership); err != nil {
		return entities.Member{}, err
	}
	u.publisher.Publish(ctx, memberEvent(a, "team.member_put", t, string(role)))
	return entities.Member{User: t.user, Role: role, CreatedAt: membership.CreatedAt}, nil
}

func (u MembersUsecase) Remove(ctx context.Context, a actor.Actor, teamSlug, login string) error {
	t, err := u.resolve(ctx, a, teamSlug, login)
	if err != nil {
		return err
	}
	if !t.exists {
		return errNotAMember
	}
	if err := u.guardLastMaintainer(ctx, a, t, ""); err != nil {
		return err
	}
	if err := u.store.DeleteMembership(ctx, t.team.ID, t.user.ID); err != nil {
		return err
	}
	u.publisher.Publish(ctx, memberEvent(a, "team.member_removed", t, ""))
	return nil
}

func (u MembersUsecase) resolve(ctx context.Context, a actor.Actor, teamSlug, login string) (target, error) {
	team, err := u.teams.FindBySlug(ctx, teamSlug)
	if err != nil {
		return target{}, err
	}
	if err := u.authz.Can(ctx, a, authz.ManageMembers, authz.Resource{TeamID: team.ID}); err != nil {
		return target{}, err
	}
	user, err := u.users.FindByLogin(ctx, login)
	if err != nil {
		return target{}, err
	}
	current, exists, err := u.store.Membership(ctx, team.ID, user.ID)
	return target{team: team, user: user, current: current, exists: exists}, err
}

func (u MembersUsecase) guardLastMaintainer(ctx context.Context, a actor.Actor, t target, newRole entities.Role) error {
	losesMaintainer := t.exists && t.current.Role == entities.RoleMaintainer && newRole != entities.RoleMaintainer
	if a.SessionAdmin() || !losesMaintainer {
		return nil
	}
	count, err := u.store.CountMaintainers(ctx, t.team.ID)
	if err != nil {
		return err
	}
	if count <= 1 {
		return errLastMaintainer
	}
	return nil
}

func memberEvent(a actor.Actor, action string, t target, role string) events.Event {
	return events.Event{
		ActorID: a.UserID, ActorLogin: a.Login, TokenID: a.Token.ID, TokenName: a.Token.Name, Action: action,
		TargetKind: events.TargetTeam, TargetID: t.team.ID, TargetName: t.team.Slug, TeamID: t.team.ID,
		Details: map[string]any{"login": t.user.Login, "role": role},
	}
}
