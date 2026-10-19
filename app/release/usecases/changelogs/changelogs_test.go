package changelogs_test

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/events"
	"github.com/stretchr/testify/require"
)

func Test_Handle_SavesPendingAndFillsInBackground(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	r2 := w.built(2, "s3", 5)
	w.compares("s1", "s3", "s2", "s3")
	w.github.pulls["s3"] = &entities.PullRef{Number: 61, Title: "Share", URL: "u"}

	require.NoError(t, w.usecase.Handle(ctx, builtEvent(r2)))

	got := w.logs.rows["r2"]
	require.Equal(t, entities.ChangelogAhead, got.State)
	require.Equal(t, "r1", got.BaseReleaseID)
	require.Equal(t, []string{"s2", "s3"}, shas(got.Commits))
	require.Equal(t, 61, got.Commits[1].PullRequest.Number)
	require.Nil(t, got.Commits[0].PullRequest)
}

func Test_Handle_RunsTheFetchThroughTheRunner(t *testing.T) {
	w := newWorld()
	r1 := w.built(1, "s1", 0)
	var queued []func()
	usecase := w.usecase.WithRunner(func(f func()) { queued = append(queued, f) })

	require.NoError(t, usecase.Handle(ctx, builtEvent(r1)))

	require.Equal(t, entities.ChangelogPending, w.logs.rows["r1"].State)
	require.Equal(t, "s1", w.logs.rows["r1"].HeadSHA)
	require.Len(t, queued, 1)
}

func Test_Handle_IgnoresOtherEvents(t *testing.T) {
	w := newWorld()
	r1 := w.built(1, "s1", 0)
	e := builtEvent(r1)
	e.Action = "release.requested"

	require.NoError(t, w.usecase.Handle(ctx, e))

	require.Empty(t, w.logs.rows)
}

func Test_Handle_SkipsReleasesWithoutCommit(t *testing.T) {
	w := newWorld()
	r1 := w.built(1, "", 0)

	require.NoError(t, w.usecase.Handle(ctx, events.Event{Action: "release.shared", TargetKind: events.TargetRelease, Details: map[string]any{"release_id": r1.ID}}))

	require.Empty(t, w.logs.rows)
}

func Test_Fill_FirstRelease(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.github.commits["s1"] = entities.Commit{SHA: "s1", Title: "feat: start", Type: "feat"}

	require.NoError(t, w.usecase.Fill(ctx, "r1"))

	got := w.logs.rows["r1"]
	require.Equal(t, entities.ChangelogFirst, got.State)
	require.Equal(t, []string{"s1"}, shas(got.Commits))
}

func Test_Fill_OutOfOrderFinishUsesFinishTime(t *testing.T) {
	w := newWorld()
	w.built(6, "s6", 0)
	w.built(7, "s7", 10)
	w.built(8, "s8", 5)
	w.compares("s6", "s8", "s8")
	w.compares("s8", "s7", "s7")

	require.NoError(t, w.usecase.Fill(ctx, "r8"))
	require.NoError(t, w.usecase.Fill(ctx, "r7"))

	require.Equal(t, "r6", w.logs.rows["r8"].BaseReleaseID)
	require.Equal(t, "r8", w.logs.rows["r7"].BaseReleaseID)
}

func Test_Fill_ReusesSamePair(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.built(2, "s2", 5)
	w.compares("s1", "s2", "s2")
	require.NoError(t, w.usecase.Fill(ctx, "r2"))
	sibling := w.built(3, "s2", 6)
	sibling.ServiceID = "other"
	w.releases.byID["r3"] = sibling
	w.built(4, "s1", 1)
	other := w.releases.byID["r4"]
	other.ServiceID = "other"
	w.releases.byID["r4"] = other

	require.NoError(t, w.usecase.Fill(ctx, "r3"))

	require.Equal(t, 1, w.github.compareCalls)
	require.Equal(t, []string{"s2"}, shas(w.logs.rows["r3"].Commits))
	require.Equal(t, "r4", w.logs.rows["r3"].BaseReleaseID)
}

func Test_Fill_DivergedKeepsCounts(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.built(2, "s2", 5)
	w.github.compares["s1...s2"] = entities.Comparison{Status: "diverged", AheadBy: 1, BehindBy: 3, Commits: []entities.Commit{{SHA: "s2"}}}

	require.NoError(t, w.usecase.Fill(ctx, "r2"))

	got := w.logs.rows["r2"]
	require.Equal(t, entities.ChangelogDiverged, got.State)
	require.Equal(t, [2]int{1, 3}, [2]int{got.AheadBy, got.BehindBy})
}

func Test_Fill_CapsPullLookupsAt30(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.built(2, "x39", 5)
	many := make([]string, 0, 40)
	for i := range 40 {
		many = append(many, fmt.Sprintf("x%d", i))
	}
	w.compares("s1", "x39", many...)

	require.NoError(t, w.usecase.Fill(ctx, "r2"))

	require.Equal(t, 30, w.github.pullCalls)
	require.Len(t, w.logs.rows["r2"].Commits, 40)
}

func Test_Fill_FailsAfterFiveAttempts(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.built(2, "s2", 5)
	w.github.down = true

	for range 4 {
		require.NoError(t, w.usecase.Fill(ctx, "r2"))
		require.Equal(t, entities.ChangelogPending, w.logs.rows["r2"].State)
	}
	require.NoError(t, w.usecase.Fill(ctx, "r2"))

	got := w.logs.rows["r2"]
	require.Equal(t, entities.ChangelogFailed, got.State)
	require.Equal(t, 5, got.Attempts)
	require.Contains(t, got.LastError, "o/api")
}

func Test_Sweep_RetriesOnlyDueRows(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.built(2, "s2", 5)
	w.built(3, "s3", 6)
	w.compares("s1", "s2", "s2")
	w.compares("s2", "s3", "s3")
	clock.WithTime(t0.Add(time.Hour))
	t.Cleanup(func() { clock.Clock = nil })
	w.logs.rows["r2"] = entities.Changelog{ReleaseID: "r2", State: entities.ChangelogPending, Attempts: 1, UpdatedAt: t0.Add(time.Hour - 3*time.Minute)}
	w.logs.rows["r3"] = entities.Changelog{ReleaseID: "r3", State: entities.ChangelogPending, Attempts: 1, UpdatedAt: t0.Add(time.Hour - time.Minute)}

	swept, err := w.usecase.Sweep(ctx)

	require.NoError(t, err)
	require.Equal(t, 1, swept)
	require.Equal(t, entities.ChangelogAhead, w.logs.rows["r2"].State)
	require.Equal(t, entities.ChangelogPending, w.logs.rows["r3"].State)
}

func Test_Get_RegisteredIsUnavailable(t *testing.T) {
	w := newWorld()
	rel := entities.Release{ID: "r9", Source: entities.SourceRegistered}

	got, err := w.usecase.Get(ctx, rel)

	require.NoError(t, err)
	require.Equal(t, entities.ChangelogUnavailable, got.State)
}

func Test_Get_ReleaseBuiltBeforeChangelogsIsUnavailable(t *testing.T) {
	w := newWorld()

	got, err := w.usecase.Get(ctx, w.built(1, "s1", 0))

	require.NoError(t, err)
	require.Equal(t, entities.ChangelogUnavailable, got.State)
	require.Equal(t, "built before changelogs existed", got.LastError)
}

func Test_Fill_SharedSiblingsFetchOnce(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.built(2, "s2", 5)
	w.compares("s1", "s2", "s2")
	w.github.gate = make(chan struct{})
	done := make(chan error, 2)

	go func() { done <- w.usecase.Fill(ctx, "r2") }()
	go func() { done <- w.usecase.Fill(ctx, "r2") }()
	time.Sleep(50 * time.Millisecond)
	close(w.github.gate)

	require.NoError(t, <-done)
	require.NoError(t, <-done)
	require.Equal(t, 1, w.github.compareCalls)
}

func Test_Fill_PullLookupFailureRetriesLater(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.built(2, "s2", 5)
	w.compares("s1", "s2", "s2")
	w.github.pullsDown = true

	require.NoError(t, w.usecase.Fill(ctx, "r2"))

	require.Equal(t, entities.ChangelogPending, w.logs.rows["r2"].State)
	require.Equal(t, 1, w.logs.rows["r2"].Attempts)
}

func Test_Fill_ClipsErrorsOnACharacterBoundary(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.built(2, "s2", 5)
	w.github.down, w.github.message = true, strings.Repeat("é", 400)

	require.NoError(t, w.usecase.Fill(ctx, "r2"))

	require.True(t, utf8.ValidString(w.logs.rows["r2"].LastError))
}

func shas(commits []entities.Commit) []string {
	out := make([]string, 0, len(commits))
	for _, c := range commits {
		out = append(out, c.SHA)
	}
	return out
}

func Test_Get_NamesTheBaseRelease(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	r2 := w.built(2, "s2", 5)
	w.logs.rows["r2"] = entities.Changelog{ReleaseID: "r2", BaseReleaseID: "r1", State: entities.ChangelogAhead}

	got, err := w.usecase.Get(ctx, r2)

	require.NoError(t, err)
	require.Equal(t, 1, got.BaseNumber)
}
