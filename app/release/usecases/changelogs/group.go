package changelogs

import (
	"context"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

func (u ChangelogsUsecase) grouped(ctx context.Context, s span, c entities.Comparison, direction string) (approval.Ships, error) {
	inBetween, err := u.inBetween(ctx, s)
	if err != nil {
		return approval.Ships{}, err
	}
	ships := approval.Ships{
		State: string(stateOf(c.Status)), Direction: direction, From: s.from.Number, To: s.to.Number,
		Truncated: c.Truncated, CompareURL: c.URL, Releases: []approval.ShipRelease{},
	}
	byRelease, loose := attribute(c.Commits, inBetween)
	for _, rel := range inBetween {
		if commits := byRelease[rel.release.ID]; len(commits) > 0 {
			ships.Releases = append(ships.Releases, approval.ShipRelease{Number: rel.release.Number, State: string(rel.log.State), Commits: commits})
		}
	}
	ships.Unattributed = loose
	ships.CommitCount, ships.PullCount = counts(append(flatten(ships.Releases), loose...))
	return ships, nil
}

type logged struct {
	release entities.Release
	log     entities.Changelog
	shas    map[string]entities.Commit
}

func (u ChangelogsUsecase) inBetween(ctx context.Context, s span) ([]logged, error) {
	low, high := min(s.from.Number, s.to.Number), max(s.from.Number, s.to.Number)
	releases, err := u.releases.ListByService(ctx, s.to.ServiceID, high+1, high-low)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(releases))
	for _, r := range releases {
		ids = append(ids, r.ID)
	}
	logs, err := u.store.FindMany(ctx, ids)
	result := make([]logged, 0, len(releases))
	for _, r := range releases {
		result = append(result, logged{release: r, log: logs[r.ID], shas: bySHA(logs[r.ID].Commits)})
	}
	return result, err
}

func attribute(commits []entities.Commit, newestFirst []logged) (map[string][]approval.ShipCommit, []approval.ShipCommit) {
	byRelease, loose := map[string][]approval.ShipCommit{}, []approval.ShipCommit{}
	for _, c := range commits {
		owner, frozen, found := oldestHolder(c.SHA, newestFirst)
		if !found {
			loose = append(loose, shipCommit(c))
			continue
		}
		byRelease[owner] = append(byRelease[owner], shipCommit(frozen))
	}
	return byRelease, loose
}

func oldestHolder(sha string, newestFirst []logged) (string, entities.Commit, bool) {
	for i := len(newestFirst) - 1; i >= 0; i-- {
		if frozen, ok := newestFirst[i].shas[sha]; ok {
			return newestFirst[i].release.ID, frozen, true
		}
	}
	return "", entities.Commit{}, false
}

func bySHA(commits []entities.Commit) map[string]entities.Commit {
	result := make(map[string]entities.Commit, len(commits))
	for _, c := range commits {
		result[c.SHA] = c
	}
	return result
}
