package members

import (
	"context"

	"github.com/adrianozp/platform/app/authz"
	"github.com/adrianozp/platform/app/team/entities"
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/events"
)

type MembershipStore interface {
	PutMembership(ctx context.Context, m entities.Membership) error
	DeleteMembership(ctx context.Context, teamID, userID string) error
	Membership(ctx context.Context, teamID, userID string) (entities.Membership, bool, error)
	MembershipsOfTeam(ctx context.Context, teamID string) ([]entities.Membership, error)
	CountMaintainers(ctx context.Context, teamID string) (int64, error)
}

type TeamFinder interface {
	FindBySlug(ctx context.Context, slug string) (entities.Team, error)
}

type UserDirectory interface {
	FindByLogin(ctx context.Context, login string) (entities.UserRef, error)
	FindByIDs(ctx context.Context, ids []string) ([]entities.UserRef, error)
}

type Authorizer interface {
	Can(ctx context.Context, a actor.Actor, action authz.Action, r authz.Resource) error
}

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

var (
	errInvalidRole    = apperror.Invalid("role must be member or maintainer")
	errLastMaintainer = apperror.New(apperror.CodeLastMaintainer, "a team needs at least one maintainer, promote someone else first")
	errNotAMember     = apperror.NotFound("this user is not a member of the team")
)

type MembersUsecase struct {
	store     MembershipStore
	teams     TeamFinder
	users     UserDirectory
	authz     Authorizer
	publisher Publisher
}

func NewMembersUsecase(store MembershipStore, teams TeamFinder, users UserDirectory, authz Authorizer, publisher Publisher) MembersUsecase {
	return MembersUsecase{store: store, teams: teams, users: users, authz: authz, publisher: publisher}
}

func (u MembersUsecase) List(ctx context.Context, teamSlug string) ([]entities.Member, error) {
	team, err := u.teams.FindBySlug(ctx, teamSlug)
	if err != nil {
		return nil, err
	}
	memberships, err := u.store.MembershipsOfTeam(ctx, team.ID)
	if err != nil {
		return nil, err
	}
	return u.withUsers(ctx, memberships)
}

func (u MembersUsecase) withUsers(ctx context.Context, memberships []entities.Membership) ([]entities.Member, error) {
	ids := make([]string, 0, len(memberships))
	for _, m := range memberships {
		ids = append(ids, m.UserID)
	}
	users, err := u.users.FindByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := map[string]entities.UserRef{}
	for _, user := range users {
		byID[user.ID] = user
	}
	result := make([]entities.Member, 0, len(memberships))
	for _, m := range memberships {
		result = append(result, entities.Member{User: byID[m.UserID], Role: m.Role, CreatedAt: m.CreatedAt})
	}
	return result, nil
}
