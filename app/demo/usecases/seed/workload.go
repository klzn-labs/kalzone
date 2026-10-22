package seed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/klzn-labs/kalzone/pkg/uuid"
)

const (
	day          = 24 * time.Hour
	history      = 12 * day
	teamAge      = 30 * day
	refreshAfter = time.Hour
)

var environments = []string{"dev", "prod"}

func (u SeedUsecase) workload(ctx context.Context, teamID string, w workload, now time.Time) error {
	s := catalog.Service{
		ID: uuid.NewUUID(), Slug: w.slug, Name: w.name, Description: w.description, OwnerTeamID: teamID,
		ImageRepository: "demo.invalid/" + w.slug, Status: catalog.StatusActive, Simulated: true, CreatedAt: now.Add(-21 * day),
	}
	if err := u.store.Services.Create(ctx, s); err != nil {
		return err
	}
	u.event(ctx, s, people[0], "service.created", "", s.CreatedAt, nil)
	if err := u.configs(ctx, s, w); err != nil {
		return err
	}
	built, err := u.releases(ctx, s, w, now)
	if err != nil {
		return err
	}
	return u.history(ctx, s, w, built, now)
}

func (u SeedUsecase) configs(ctx context.Context, s catalog.Service, w workload) error {
	for _, env := range environments {
		rev := envconfig.Initial(s.ID, env)
		rev.ID, rev.Number, rev.Env, rev.AuthorID = uuid.NewUUID(), 1, w.env, people[0].id
		rev.Message, rev.CreatedAt = "first config", s.CreatedAt
		rev.Spec.Host = fmt.Sprintf("%s-%s.demo.invalid", w.slug, env)
		if err := u.store.Configs.Insert(ctx, rev); err != nil {
			return err
		}
	}
	return nil
}

func (u SeedUsecase) releases(ctx context.Context, s catalog.Service, w workload, now time.Time) ([]release.Release, error) {
	built := make([]release.Release, 0, w.releases)
	for n := 1; n <= w.releases; n++ {
		rel, err := u.store.Releases.Create(ctx, releaseOf(s, n, releasedAt(w, n, now)))
		if err != nil {
			return nil, err
		}
		built = append(built, rel)
	}
	return built, u.changelogs(ctx, w, built)
}

func releasedAt(w workload, n int, now time.Time) time.Time {
	every := history / time.Duration(w.releases)
	return now.Add(-time.Duration(w.releases-n+1) * every).Truncate(time.Minute)
}

func releaseOf(s catalog.Service, n int, at time.Time) release.Release {
	sha := shaOf(fmt.Sprintf("%s-%d", s.Slug, n))
	return release.Release{
		ID: uuid.NewUUID(), ServiceID: s.ID, Source: release.SourceBuild, GitRef: "main", CommitSHA: sha[:40],
		Image: s.ImageRepository, Tag: fmt.Sprintf("r%d-%s", n, sha[:7]), Digest: "sha256:" + sha,
		Status: release.StatusBuilt, Trigger: release.TriggerPush, RequestedBy: people[n%len(people)].id,
		CreatedAt: at.Add(-4 * time.Minute), FinishedAt: &at,
	}
}

func shaOf(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(sum[:])
}

func (u SeedUsecase) changelogs(ctx context.Context, w workload, built []release.Release) error {
	for i := max(1, len(built)-4); i < len(built); i++ {
		if err := u.store.Changelogs.Save(ctx, changelogOf(w, built[i-1], built[i])); err != nil {
			return err
		}
	}
	return nil
}

func changelogOf(w workload, base, head release.Release) release.Changelog {
	commits := make([]release.Commit, 0, 2)
	for k := range 2 {
		title := titles[(head.Number*2+k)%len(titles)]
		sha := shaOf(fmt.Sprintf("%s-%d-%d", w.slug, head.Number, k))[:40]
		commits = append(commits, release.Commit{SHA: sha, Title: title, Author: people[(head.Number+k)%len(people)].login, CommittedAt: head.CreatedAt})
	}
	return release.Changelog{
		ReleaseID: head.ID, BaseReleaseID: base.ID, BaseSHA: base.CommitSHA, HeadSHA: head.CommitSHA,
		State: release.ChangelogAhead, AheadBy: len(commits), Commits: commits, UpdatedAt: *head.FinishedAt,
	}
}

func (u SeedUsecase) event(ctx context.Context, s catalog.Service, p person, action, env string, at time.Time, details map[string]any) {
	_ = u.store.Events.Insert(ctx, auditOf(s, p, action, env, at, details))
}

func targetOf(s catalog.Service) (string, string, string) {
	return events.TargetService, s.ID, s.Slug
}
