package changelogs

import (
	"context"
	"log"

	"github.com/klzn-labs/kalzone/app/release/entities"
)

type target struct {
	release    entities.Release
	repository string
	base       entities.Release
	hasBase    bool
}

func (u ChangelogsUsecase) logFill(ctx context.Context, releaseID string) {
	ctx, cancel := context.WithTimeout(ctx, entities.FetchTimeout)
	defer cancel()
	if err := u.Fill(ctx, releaseID); err != nil {
		log.Printf("changelog of release %s: %v", releaseID, err)
	}
}

func (u ChangelogsUsecase) Fill(ctx context.Context, releaseID string) error {
	t, err := u.targetOf(ctx, releaseID)
	if err != nil {
		return err
	}
	previous, _, err := u.store.Find(ctx, releaseID)
	if err != nil {
		return err
	}
	c, err := u.flights.do(pairKey(t), func() (entities.Changelog, error) { return u.compute(ctx, t) })
	c = rebased(c, t)
	if err != nil {
		return u.store.Save(ctx, retried(previous, t, err))
	}
	c.UpdatedAt = now()
	return u.store.Save(ctx, c)
}

func (u ChangelogsUsecase) targetOf(ctx context.Context, releaseID string) (target, error) {
	rel, err := u.releases.FindByID(ctx, releaseID)
	if err != nil {
		return target{}, err
	}
	service, err := u.services.FindServiceByID(ctx, rel.ServiceID)
	if err != nil {
		return target{}, err
	}
	t := target{release: rel, repository: service.GitHubRepo}
	if rel.FinishedAt != nil {
		t.base, t.hasBase, err = u.releases.PreviousBuilt(ctx, rel.ServiceID, rel.ID, *rel.FinishedAt)
	}
	return t, err
}

func (u ChangelogsUsecase) compute(ctx context.Context, t target) (entities.Changelog, error) {
	if !t.hasBase {
		return u.first(ctx, t)
	}
	if reused, found, err := u.store.ByPair(ctx, t.base.CommitSHA, t.release.CommitSHA); err != nil || found {
		return rebased(reused, t), err
	}
	comparison, err := u.github.Compare(ctx, t.repository, t.base.CommitSHA, t.release.CommitSHA)
	if err != nil {
		return entities.Changelog{}, err
	}
	c := rebased(fromComparison(comparison), t)
	c.Commits, err = u.withPulls(ctx, t.repository, c.Commits)
	return c, err
}

func (u ChangelogsUsecase) first(ctx context.Context, t target) (entities.Changelog, error) {
	commit, err := u.github.Commit(ctx, t.repository, t.release.CommitSHA)
	if err != nil {
		return entities.Changelog{}, err
	}
	c := entities.Changelog{ReleaseID: t.release.ID, HeadSHA: t.release.CommitSHA, State: entities.ChangelogFirst}
	c.Commits, err = u.withPulls(ctx, t.repository, []entities.Commit{commit})
	return c, err
}

func (u ChangelogsUsecase) withPulls(ctx context.Context, repository string, commits []entities.Commit) ([]entities.Commit, error) {
	for i := range commits[:min(len(commits), entities.MaxPullLookups)] {
		pull, err := u.github.PullFor(ctx, repository, commits[i].SHA)
		if err != nil {
			return nil, err
		}
		commits[i].PullRequest = pull
	}
	return commits, nil
}

func pairKey(t target) string {
	return t.repository + "@" + t.base.CommitSHA + "..." + t.release.CommitSHA
}
