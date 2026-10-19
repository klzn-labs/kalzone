package releases

import (
	"testing"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/stretchr/testify/require"
)

const notFound = `dispatch failed: github POST /repos/a/b/actions/workflows/platform-build.yml/dispatches: 422 {"message":"... workflow was not found."}`

func Test_WiringOf_ReadsTheLatestFinishedBuild(t *testing.T) {
	linked := catalog.Service{GitHubRepo: "a/b"}
	built := release.Release{Source: release.SourceBuild, Status: release.StatusBuilt}
	refused := release.Release{Source: release.SourceBuild, Status: release.StatusFailed, Failure: notFound}
	building := release.Release{Source: release.SourceBuild, Status: release.StatusBuilding}
	registered := release.Release{Source: release.SourceRegistered, Status: release.StatusBuilt}
	cases := map[string]struct {
		service catalog.Service
		list    []release.Release
	}{
		"repository not linked":     {catalog.Service{}, []release.Release{built}},
		"no builds yet":             {linked, []release.Release{registered}},
		"workflow wired up":         {linked, []release.Release{building, built, refused}},
		"last build couldn't start": {linked, []release.Release{refused, built}},
	}
	for want, c := range cases {
		require.Equal(t, want, wiringOf(c.service, c.list).Label, want)
	}
}

func Test_FailureSummary_ShortensDispatchErrors(t *testing.T) {
	require.Equal(t, "Build workflow not found", failureSummary(notFound))
	require.Equal(t, "GitHub refused to start the build", failureSummary("dispatch failed: github POST ...: 404"))
	require.Equal(t, "tests failed", failureSummary("tests failed"))
}
