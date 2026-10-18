package releases_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

const (
	firstID = "0b3c5d1e-2f4a-4b6c-8d9e-0a1b2c3d4e5f"
	shaA    = "abc1234000000000000000000000000000000000"
	shaA2   = "abc1234999999999999999999999999999999999"
	shaB    = "def5678000000000000000000000000000000000"
)

func stored(f fixture, id, serviceID string, status entities.Status, sha, tag string) {
	_, _ = f.store.Create(ctx, entities.Release{ID: id, ServiceID: serviceID, Status: status, CommitSHA: sha, Tag: tag})
}

func Test_Resolve_NamesReleasesTheWayPeopleDo(t *testing.T) {
	f := newFixture(t, false)
	stored(f, firstID, api.ID, entities.StatusBuilt, shaA, "1.0.0")
	stored(f, "a2", api.ID, entities.StatusBuilt, shaB, "")
	stored(f, "a3", api.ID, entities.StatusBuilding, "", "")
	stored(f, "w1", worker.ID, entities.StatusBuilt, shaB, "")
	cases := map[string]string{firstID: firstID, "latest": "a2", "r3": "a3", "sha:abc1234": firstID, "sha:DEF5678": "a2", "1.0.0": firstID}

	for ref, want := range cases {
		rel, err := f.usecase.Resolve(ctx, "api", ref)
		require.NoError(t, err, ref)
		require.Equal(t, want, rel.ID, ref)
	}
}

func Test_Resolve_RefusesAmbiguousAndForeignReferences(t *testing.T) {
	f := newFixture(t, false)
	stored(f, "a1", api.ID, entities.StatusBuilt, shaA, "")
	stored(f, "a2", api.ID, entities.StatusBuilt, shaA2, "")
	stored(f, firstID, worker.ID, entities.StatusBuilt, shaB, "")

	_, ambiguous := f.usecase.Resolve(ctx, "api", "sha:abc1234")
	_, foreign := f.usecase.Resolve(ctx, "api", firstID)
	_, missing := f.usecase.Resolve(ctx, "api", "r9")
	_, nothingBuilt := f.usecase.Resolve(ctx, "web", "latest")

	require.ErrorIs(t, ambiguous, apperror.Invalid(""))
	require.ErrorIs(t, foreign, apperror.NotFound(""))
	require.Equal(t, "list releases with GET /api/v1/releases?service=api", foreign.(*apperror.AppError).Hint)
	require.ErrorIs(t, missing, apperror.NotFound(""))
	require.ErrorIs(t, nothingBuilt, apperror.NotFound(""))
}

func Test_Resolve_RefusesNonHexCommitPrefixBeforeTouchingTheStore(t *testing.T) {
	f := newFixture(t, false)
	stored(f, "a1", api.ID, entities.StatusBuilt, shaA, "")

	_, err := f.usecase.Resolve(ctx, "api", "sha:abc%%%%")

	require.ErrorIs(t, err, apperror.Invalid(""))
	require.NotEmpty(t, err.(*apperror.AppError).Hint)
}

func Test_ResolveFor_HidesServicesOfOtherTeams(t *testing.T) {
	f := newFixture(t, false)
	stored(f, "a1", api.ID, entities.StatusBuilt, shaA, "")
	outsider := actor.Actor{UserID: "zed", Token: actor.Token{ID: "k1", TeamIDs: []string{"t9"}}}

	_, hidden := f.usecase.ResolveFor(ctx, outsider, "api", "latest")
	rel, err := f.usecase.ResolveFor(ctx, ana, "api", "latest")

	require.Equal(t, "service not found", hidden.(*apperror.AppError).Message)
	require.NoError(t, err)
	require.Equal(t, "a1", rel.ID)
}
