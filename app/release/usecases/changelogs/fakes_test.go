package changelogs_test

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/app/release/usecases/changelogs"
	"github.com/klzn-labs/kalzone/pkg/events"
)

var ctx = context.Background()

type logStore struct {
	mu   sync.Mutex
	rows map[string]entities.Changelog
}

func (s *logStore) Save(_ context.Context, c entities.Changelog) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows[c.ReleaseID] = c
	return nil
}

func (s *logStore) Find(_ context.Context, id string) (entities.Changelog, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.rows[id]
	return c, ok, nil
}

func (s *logStore) FindMany(_ context.Context, ids []string) (map[string]entities.Changelog, error) {
	out := map[string]entities.Changelog{}
	for _, id := range ids {
		if c, ok := s.rows[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

func (s *logStore) Pending(_ context.Context) ([]entities.Changelog, error) {
	out := []entities.Changelog{}
	for _, c := range s.rows {
		if c.State == entities.ChangelogPending {
			out = append(out, c)
		}
	}
	return out, nil
}

func (s *logStore) ByPair(_ context.Context, base, head string) (entities.Changelog, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.rows {
		if c.BaseSHA == base && c.HeadSHA == head && c.State != entities.ChangelogPending && c.State != entities.ChangelogFailed {
			return c, true, nil
		}
	}
	return entities.Changelog{}, false, nil
}

type releaseStore struct{ byID map[string]entities.Release }

func (s *releaseStore) FindByID(_ context.Context, id string) (entities.Release, error) {
	rel, ok := s.byID[id]
	if !ok {
		return entities.Release{}, errors.New("release not found")
	}
	return rel, nil
}

func (s *releaseStore) PreviousBuilt(_ context.Context, serviceID, excludeID string, before time.Time) (entities.Release, bool, error) {
	var best entities.Release
	for _, r := range s.byID {
		if r.ServiceID != serviceID || r.ID == excludeID || r.CommitSHA == "" || !r.FinishedAt.Before(before) {
			continue
		}
		if best.ID == "" || r.FinishedAt.After(*best.FinishedAt) {
			best = r
		}
	}
	return best, best.ID != "", nil
}

func (s *releaseStore) ListByService(_ context.Context, serviceID string, before, limit int) ([]entities.Release, error) {
	out := []entities.Release{}
	for _, r := range s.byID {
		if r.ServiceID == serviceID && r.Number < before {
			out = append(out, r)
		}
	}
	slices.SortFunc(out, func(a, b entities.Release) int { return b.Number - a.Number })
	return out[:min(limit, len(out))], nil
}

type services struct{}

func (services) FindServiceByID(_ context.Context, id string) (entities.ServiceRef, error) {
	return entities.ServiceRef{ID: id, Slug: "api", GitHubRepo: "o/api"}, nil
}

type github struct {
	mu           sync.Mutex
	compares     map[string]entities.Comparison
	commits      map[string]entities.Commit
	pulls        map[string]*entities.PullRef
	down         bool
	pullsDown    bool
	message      string
	gate         chan struct{}
	compareCalls int
	pullCalls    int
}

func (g *github) Compare(_ context.Context, repo, base, head string) (entities.Comparison, error) {
	if g.gate != nil {
		<-g.gate
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.compareCalls++
	c, ok := g.compares[base+"..."+head]
	if g.down || !ok {
		return entities.Comparison{}, errors.New("github GET /repos/" + repo + "/compare: 502 " + g.message)
	}
	return c, nil
}

func (g *github) Commit(_ context.Context, _, sha string) (entities.Commit, error) {
	c, ok := g.commits[sha]
	if g.down || !ok {
		return entities.Commit{}, errors.New("github: 502")
	}
	return c, nil
}

func (g *github) PullFor(_ context.Context, _, sha string) (*entities.PullRef, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pullCalls++
	if g.pullsDown {
		return nil, errors.New("github: 403 rate limited")
	}
	return g.pulls[sha], nil
}

type world struct {
	logs     *logStore
	releases *releaseStore
	github   *github
	usecase  changelogs.ChangelogsUsecase
}

func newWorld() *world {
	w := &world{
		logs:     &logStore{rows: map[string]entities.Changelog{}},
		releases: &releaseStore{byID: map[string]entities.Release{}},
		github:   &github{compares: map[string]entities.Comparison{}, commits: map[string]entities.Commit{}, pulls: map[string]*entities.PullRef{}},
	}
	w.usecase = changelogs.NewChangelogsUsecase(w.logs, w.releases, services{}, w.github).WithRunner(func(f func()) { f() })
	return w
}

var t0 = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

func (w *world) built(number int, sha string, finishedMinute int) entities.Release {
	at := t0.Add(time.Duration(finishedMinute) * time.Minute)
	rel := entities.Release{
		ID: "r" + itoa(number), ServiceID: "svc", Number: number, CommitSHA: sha,
		Status: entities.StatusBuilt, Source: entities.SourceBuild, FinishedAt: &at,
	}
	w.releases.byID[rel.ID] = rel
	return rel
}

func (w *world) compares(base, head string, shas ...string) {
	commits := make([]entities.Commit, 0, len(shas))
	for _, sha := range shas {
		commits = append(commits, entities.Commit{SHA: sha, Title: "feat: " + sha, Type: "feat", Author: "ana"})
	}
	w.github.compares[base+"..."+head] = entities.Comparison{Status: "ahead", AheadBy: len(shas), Commits: commits, URL: "https://github.com/o/api/compare/" + base + "..." + head}
}

func builtEvent(rel entities.Release) events.Event {
	return events.Event{Action: "release.built", TargetKind: events.TargetRelease, TargetID: rel.ServiceID, Details: map[string]any{"release_id": rel.ID}}
}

func itoa(n int) string {
	return string(rune('0' + n))
}
