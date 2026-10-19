package releases

import (
	"testing"

	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/stretchr/testify/require"
)

func Test_PullGroups_KeepsCommitsOfOnePullTogether(t *testing.T) {
	pr := &release.PullRef{Number: 59, Title: "Fixes"}
	commits := []release.Commit{
		{SHA: "a", Title: "feat: x"},
		{SHA: "b", PullRequest: pr},
		{SHA: "c"},
		{SHA: "d", PullRequest: pr},
	}

	groups := pullGroups(commits)

	require.Len(t, groups, 3)
	require.Nil(t, groups[0].Pull)
	require.Equal(t, []string{"b", "d"}, []string{groups[1].Commits[0].SHA, groups[1].Commits[1].SHA})
	require.Equal(t, "c", groups[2].Commits[0].SHA)
}

func Test_ChangesLabel_SaysWhatTheRowCarries(t *testing.T) {
	numbers := map[string]int{"r7": 7}
	cases := map[string]release.Changelog{
		"5 commits since r7":               {State: release.ChangelogAhead, BaseReleaseID: "r7", Commits: make([]release.Commit, 5)},
		"1 commit since r7":                {State: release.ChangelogDiverged, BaseReleaseID: "r7", Commits: make([]release.Commit, 1)},
		"3 commits since the last release": {State: release.ChangelogAhead, BaseReleaseID: "old", Commits: make([]release.Commit, 3)},
		"first release":                    {State: release.ChangelogFirst},
		"reading GitHub…":                  {State: release.ChangelogPending},
		"changelog failed":                 {State: release.ChangelogFailed},
		"":                                 {State: release.ChangelogUnavailable},
	}
	for want, c := range cases {
		require.Equal(t, want, changesLabel(c, numbers), want)
	}
}
