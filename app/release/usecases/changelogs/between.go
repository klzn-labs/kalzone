package changelogs

import (
	"context"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type span struct {
	from, to   entities.Release
	repository string
}

func (u ChangelogsUsecase) Between(ctx context.Context, fromReleaseID, toReleaseID string) (approval.Ships, error) {
	s, err := u.spanOf(ctx, fromReleaseID, toReleaseID)
	if err != nil {
		return approval.Ships{}, err
	}
	if fromReleaseID == "" {
		return u.firstShips(ctx, s.to)
	}
	ships := approval.Ships{
		State: string(entities.ChangelogUnavailable), Direction: approval.ShipsForward,
		From: s.from.Number, To: s.to.Number, Releases: []approval.ShipRelease{},
	}
	if s.from.CommitSHA == "" || s.to.CommitSHA == "" {
		return ships, nil
	}
	comparison, direction, err := u.directed(ctx, s)
	if err != nil {
		ships.State, ships.Error = string(entities.ChangelogFailed), clip(err.Error())
		return ships, nil
	}
	grouped, err := u.grouped(ctx, s, comparison, direction)
	if err != nil || comparison.Status != "diverged" {
		return grouped, err
	}
	return u.withLeaving(ctx, s, grouped), nil
}

func (u ChangelogsUsecase) withLeaving(ctx context.Context, s span, ships approval.Ships) approval.Ships {
	leaving, err := u.github.Compare(ctx, s.repository, s.to.CommitSHA, s.from.CommitSHA)
	if err != nil {
		ships.Error = clip(err.Error())
		return ships
	}
	ships.Leaving = shipCommits(leaving.Commits)
	return ships
}

func (u ChangelogsUsecase) spanOf(ctx context.Context, fromID, toID string) (span, error) {
	to, err := u.releases.FindByID(ctx, toID)
	if err != nil {
		return span{}, err
	}
	service, err := u.services.FindServiceByID(ctx, to.ServiceID)
	if err != nil || fromID == "" {
		return span{to: to, repository: service.GitHubRepo}, err
	}
	from, err := u.releases.FindByID(ctx, fromID)
	return span{from: from, to: to, repository: service.GitHubRepo}, err
}

func (u ChangelogsUsecase) directed(ctx context.Context, s span) (entities.Comparison, string, error) {
	forward, err := u.github.Compare(ctx, s.repository, s.from.CommitSHA, s.to.CommitSHA)
	if err != nil || forward.Status != "behind" {
		return forward, approval.ShipsForward, err
	}
	backward, err := u.github.Compare(ctx, s.repository, s.to.CommitSHA, s.from.CommitSHA)
	return backward, approval.ShipsBackward, err
}

func (u ChangelogsUsecase) firstShips(ctx context.Context, to entities.Release) (approval.Ships, error) {
	c, err := u.Get(ctx, to)
	release := approval.ShipRelease{Number: to.Number, State: string(c.State), Commits: shipCommits(c.Commits)}
	ships := approval.Ships{State: string(entities.ChangelogFirst), Direction: approval.ShipsForward, To: to.Number, Releases: []approval.ShipRelease{release}}
	ships.CommitCount, ships.PullCount = counts(release.Commits)
	return ships, err
}
