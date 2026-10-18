package source

import (
	"context"

	"github.com/klzn-labs/kalzone/app/team/entities"
)

type MembershipReader interface {
	Membership(ctx context.Context, teamID, userID string) (entities.Membership, bool, error)
	MembershipsOfUser(ctx context.Context, userID string) ([]entities.Membership, error)
}

type LocalTeamSource struct {
	memberships MembershipReader
}

func NewLocalTeamSource(memberships MembershipReader) LocalTeamSource {
	return LocalTeamSource{memberships: memberships}
}

func (s LocalTeamSource) RoleOf(ctx context.Context, teamID, userID string) (string, error) {
	m, found, err := s.memberships.Membership(ctx, teamID, userID)
	if err != nil || !found {
		return "", err
	}
	return string(m.Role), nil
}

func (s LocalTeamSource) MembershipsOf(ctx context.Context, userID string) ([]entities.Membership, error) {
	return s.memberships.MembershipsOfUser(ctx, userID)
}

func (s LocalTeamSource) TeamIDsOf(ctx context.Context, userID string) ([]string, error) {
	memberships, err := s.memberships.MembershipsOfUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(memberships))
	for _, m := range memberships {
		ids = append(ids, m.TeamID)
	}
	return ids, nil
}
