package web

import (
	"testing"

	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/stretchr/testify/require"
)

func Test_ShipLines_FoldsCommitsOfOnePull(t *testing.T) {
	commits := []approval.ShipCommit{
		{SHA: "a", Title: "feat: x", Type: "feat", PullNumber: 61, PullTitle: "Share"},
		{SHA: "b", Title: "fix: y", Type: "fix", PullNumber: 59, PullTitle: "Fixes"},
		{SHA: "c", Title: "fix: z", Type: "fix", PullNumber: 59, PullTitle: "Fixes"},
		{SHA: "d", Title: "tidy", Type: "other"},
	}

	lines := shipLines(commits)

	require.Equal(t, []ShipLine{
		{Title: "Share", Type: "feat", SHA: "a", PullNumber: 61, Commits: 1},
		{Title: "Fixes", Type: "fix", SHA: "b", PullNumber: 59, Commits: 2},
		{Title: "tidy", Type: "other", SHA: "d", Commits: 1},
	}, lines)
}
