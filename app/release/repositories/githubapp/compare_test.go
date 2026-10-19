package githubapp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/app/release/repositories/githubapp"
	"github.com/klzn-labs/kalzone/internal/fakegithub"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

func appClient(t *testing.T) (githubapp.Client, *fakegithub.Server) {
	t.Helper()
	server := fakegithub.New()
	t.Cleanup(server.Close)
	client, err := githubapp.NewClient(config.Config{GitHub: config.GitHub{AppID: 1, AppPrivateKey: privateKeyPEM(t), APIURL: server.URL}})
	require.NoError(t, err)
	return client, server
}

func Test_Compare_ReadsCommitsNewestFirst(t *testing.T) {
	client, server := appClient(t)
	server.WillCompare("o/api", "aaa", "ccc", fakegithub.Comparison{Status: "ahead", AheadBy: 2, Commits: []fakegithub.Commit{
		{SHA: "bbb", Message: "fix(auth): refresh once\n\nbody", Login: "c4rras"},
		{SHA: "ccc", Message: "feat: share lists", Login: "adrianozp"},
	}})

	got, err := client.Compare(context.Background(), "o/api", "aaa", "ccc")

	require.NoError(t, err)
	require.Equal(t, "ahead", got.Status)
	require.Equal(t, 2, got.AheadBy)
	require.False(t, got.Truncated)
	require.Equal(t, "https://github.com/o/api/compare/aaa", got.URL)
	require.Equal(t, []string{"ccc", "bbb"}, []string{got.Commits[0].SHA, got.Commits[1].SHA})
	require.Equal(t, "fix", got.Commits[1].Type)
	require.Equal(t, "fix(auth): refresh once", got.Commits[1].Title)
	require.Equal(t, "c4rras", got.Commits[1].Author)
	require.False(t, got.Commits[0].CommittedAt.IsZero())
}

func Test_Compare_FlagsTruncation(t *testing.T) {
	client, server := appClient(t)
	server.WillCompare("o/api", "aaa", "ccc", fakegithub.Comparison{Status: "ahead", AheadBy: 300, Total: 300, Commits: []fakegithub.Commit{{SHA: "ccc", Message: "x"}}})

	got, err := client.Compare(context.Background(), "o/api", "aaa", "ccc")

	require.NoError(t, err)
	require.True(t, got.Truncated)
}

func Test_Compare_FallsBackToTheGitAuthorName(t *testing.T) {
	client, server := appClient(t)
	server.WillCompare("o/api", "aaa", "ccc", fakegithub.Comparison{Status: "ahead", Commits: []fakegithub.Commit{{SHA: "ccc", Message: "x"}}})

	got, err := client.Compare(context.Background(), "o/api", "aaa", "ccc")

	require.NoError(t, err)
	require.Equal(t, "Git ", got.Commits[0].Author)
}

func Test_Compare_ErrorNamesTheRepo(t *testing.T) {
	client, server := appClient(t)
	server.FailHistory("o/api", true)

	_, err := client.Compare(context.Background(), "o/api", "aaa", "ccc")

	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "o/api"), err.Error())
}

func Test_PullFor_ReturnsTheMergedPullRequest(t *testing.T) {
	client, server := appClient(t)
	server.WillCommit("o/api", fakegithub.Commit{SHA: "ccc", Message: "x", PullNumber: 61, PullTitle: "Share lists"})

	pull, err := client.PullFor(context.Background(), "o/api", "ccc")

	require.NoError(t, err)
	require.Equal(t, &entities.PullRef{Number: 61, Title: "Share lists", URL: "https://github.com/o/api/pull/61"}, pull)
}

func Test_PullFor_ReturnsNilWithoutPR(t *testing.T) {
	client, server := appClient(t)
	server.WillCommit("o/api", fakegithub.Commit{SHA: "ccc", Message: "x"})

	pull, err := client.PullFor(context.Background(), "o/api", "ccc")

	require.NoError(t, err)
	require.Nil(t, pull)
}

func Test_Commit_ReadsOneCommit(t *testing.T) {
	client, server := appClient(t)
	server.WillCommit("o/api", fakegithub.Commit{SHA: "ccc", Message: "feat: first", Login: "ana"})

	commit, err := client.Commit(context.Background(), "o/api", "ccc")

	require.NoError(t, err)
	require.Equal(t, "feat: first", commit.Title)
	require.Equal(t, "ana", commit.Author)
}
