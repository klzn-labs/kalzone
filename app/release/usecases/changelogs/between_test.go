package changelogs_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

func (w *world) frozen(id string, pulls map[string]int, shas ...string) {
	commits := make([]entities.Commit, 0, len(shas))
	for _, sha := range shas {
		c := entities.Commit{SHA: sha, Title: "feat: " + sha, Type: "feat"}
		if n, ok := pulls[sha]; ok {
			c.PullRequest = &entities.PullRef{Number: n, Title: "PR", URL: "u"}
		}
		commits = append(commits, c)
	}
	w.logs.rows[id] = entities.Changelog{ReleaseID: id, State: entities.ChangelogAhead, Commits: commits}
}

func releaseNumbers(s approval.Ships) []int {
	out := []int{}
	for _, r := range s.Releases {
		out = append(out, r.Number)
	}
	return out
}

func (w *world) fiveToEight() {
	w.built(5, "s5", 0)
	w.built(6, "s6", 1)
	w.built(7, "s7", 2)
	w.built(8, "s8", 3)
	w.frozen("r6", map[string]int{"s6": 56}, "s6")
	w.frozen("r7", map[string]int{"s7": 57, "x7": 57}, "s7", "x7")
	w.frozen("r8", map[string]int{"s8": 58}, "s8")
}

func Test_Between_ForwardGroupsByRelease(t *testing.T) {
	w := newWorld()
	w.fiveToEight()
	w.compares("s5", "s8", "s8", "x7", "s7", "s6")

	ships, err := w.usecase.Between(ctx, "r5", "r8")

	require.NoError(t, err)
	require.Equal(t, "ahead", ships.State)
	require.Equal(t, "forward", ships.Direction)
	require.Equal(t, [2]int{5, 8}, [2]int{ships.From, ships.To})
	require.Equal(t, []int{8, 7, 6}, releaseNumbers(ships))
	require.Len(t, ships.Releases[1].Commits, 2)
	require.Equal(t, 57, ships.Releases[1].Commits[0].PullNumber)
	require.Equal(t, [2]int{4, 3}, [2]int{ships.CommitCount, ships.PullCount})
}

func Test_Between_FeatureBranchReleaseInBetween(t *testing.T) {
	w := newWorld()
	w.fiveToEight()
	w.compares("s5", "s8", "s8", "s6", "loose")

	ships, err := w.usecase.Between(ctx, "r5", "r8")

	require.NoError(t, err)
	require.Equal(t, []int{8, 6}, releaseNumbers(ships))
	require.Equal(t, []string{"loose"}, commitSHAs(ships.Unattributed))
	require.Equal(t, 3, ships.CommitCount)
}

func Test_Between_RollbackListsWhatLeaves(t *testing.T) {
	w := newWorld()
	w.fiveToEight()
	w.github.compares["s8...s5"] = entities.Comparison{Status: "behind", BehindBy: 3}
	w.compares("s5", "s8", "s8", "s7", "s6")

	ships, err := w.usecase.Between(ctx, "r8", "r5")

	require.NoError(t, err)
	require.Equal(t, "backward", ships.Direction)
	require.Equal(t, [2]int{8, 5}, [2]int{ships.From, ships.To})
	require.Equal(t, []int{8, 7, 6}, releaseNumbers(ships))
}

func Test_Between_NoLiveIsFirst(t *testing.T) {
	w := newWorld()
	w.built(1, "s1", 0)
	w.frozen("r1", nil, "s1")

	ships, err := w.usecase.Between(ctx, "", "r1")

	require.NoError(t, err)
	require.Equal(t, "first", ships.State)
	require.Equal(t, []int{1}, releaseNumbers(ships))
	require.Equal(t, 1, ships.CommitCount)
}

func Test_Between_RegisteredIsUnavailable(t *testing.T) {
	w := newWorld()
	w.built(1, "", 0)
	w.built(2, "s2", 1)

	ships, err := w.usecase.Between(ctx, "r1", "r2")

	require.NoError(t, err)
	require.Equal(t, "unavailable", ships.State)
	require.Zero(t, w.github.compareCalls)
	require.NotNil(t, ships.Releases)
}

func Test_Between_DivergedListsWhatShipsAndWhatLeaves(t *testing.T) {
	w := newWorld()
	w.fiveToEight()
	w.github.compares["s5...s8"] = entities.Comparison{Status: "diverged", AheadBy: 1, BehindBy: 1, Commits: []entities.Commit{{SHA: "s8"}}}
	w.github.compares["s8...s5"] = entities.Comparison{Status: "diverged", AheadBy: 1, BehindBy: 1, Commits: []entities.Commit{{SHA: "hotfix", Title: "fix: hot"}}}

	ships, err := w.usecase.Between(ctx, "r5", "r8")

	require.NoError(t, err)
	require.Equal(t, "diverged", ships.State)
	require.Equal(t, []int{8}, releaseNumbers(ships))
	require.Equal(t, []string{"hotfix"}, commitSHAs(ships.Leaving))
}

func Test_Between_GitHubDownIsFailedNotError(t *testing.T) {
	w := newWorld()
	w.fiveToEight()
	w.github.down = true

	ships, err := w.usecase.Between(ctx, "r5", "r8")

	require.NoError(t, err)
	require.Equal(t, "failed", ships.State)
	require.Contains(t, ships.Error, "o/api")
	require.NotNil(t, ships.Releases)
}

func commitSHAs(commits []approval.ShipCommit) []string {
	out := []string{}
	for _, c := range commits {
		out = append(out, c.SHA)
	}
	return out
}
