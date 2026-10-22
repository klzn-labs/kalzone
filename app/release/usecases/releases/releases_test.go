package releases_test

import (
	"context"
	"testing"
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/stretchr/testify/require"
)

var ctx = context.Background()

const sha = "0123456789abcdef0123456789abcdef01234567"

func Test_Request_DispatchesTheBuildAndMarksItBuilding(t *testing.T) {
	f := newFixture(t, false)

	rel, err := f.usecase.Request(ctx, ana, "api", "main")

	require.NoError(t, err)
	require.Equal(t, entities.StatusBuilding, rel.Status)
	require.Equal(t, entities.StatusBuilding, f.store.byID[rel.ID].Status)
	request := f.builder.requests[0]
	require.Equal(t, "o/api", request.Repository)
	require.Equal(t, "platform-build.yml", request.Workflow)
	require.Equal(t, "main", request.Ref)
	require.Equal(t, map[string]string{"release_id": rel.ID, "image": "docker.io/o/api", "platform_url": "https://platform.test"}, request.Inputs)
	require.Equal(t, []string{"release.requested"}, f.events.actions)
}

func Test_Request_RefusesBadInputAndOutsiders(t *testing.T) {
	f := newFixture(t, false)
	denied := newFixture(t, true)

	_, badRef := f.usecase.Request(ctx, ana, "api", "main; rm -rf /")
	_, noRepo := f.usecase.Request(ctx, ana, "bare", "main")
	_, forbidden := denied.usecase.Request(ctx, ana, "api", "main")
	_, simulated := f.usecase.Request(ctx, ana, "demo-shop", "main")

	require.ErrorIs(t, badRef, apperror.Invalid(""))
	require.ErrorIs(t, noRepo, apperror.Invalid(""))
	require.ErrorIs(t, forbidden, apperror.Forbidden(""))
	require.ErrorIs(t, simulated, apperror.Conflict(""))
	require.Empty(t, f.builder.requests)
}

func Test_Request_AFailedDispatchFailsTheRelease(t *testing.T) {
	f := newFixture(t, false)
	f.builder.err = errBoom

	rel, err := f.usecase.Request(ctx, ana, "api", "main")

	require.ErrorIs(t, err, apperror.New(apperror.CodeUpstream, ""))
	require.Equal(t, entities.StatusFailed, rel.Status)
	require.Contains(t, f.store.byID[rel.ID].Failure, "workflow not found")
}

func Test_Built_FinishesTheReleaseForTheRightRepositoryOnly(t *testing.T) {
	f := newFixture(t, false)
	rel, _ := f.usecase.Request(ctx, ana, "api", "main")
	report := entities.BuildReport{Digest: "sha256:" + digestHex, CommitSHA: sha, Tag: "r1", RunURL: "https://github.com/o/api/actions/runs/1"}

	_, intruder := f.usecase.Built(ctx, entities.BuildCaller{Repository: "evil/repo"}, rel.ID, report)
	built, err := f.usecase.Built(ctx, entities.BuildCaller{Repository: "o/api"}, rel.ID, report)
	_, again := f.usecase.Built(ctx, entities.BuildCaller{Repository: "o/api"}, rel.ID, report)

	require.ErrorIs(t, intruder, apperror.Forbidden(""))
	require.NoError(t, err)
	require.Equal(t, entities.StatusBuilt, built.Status)
	require.Equal(t, "docker.io/o/api@sha256:"+digestHex, built.Reference())
	require.Equal(t, sha, built.CommitSHA)
	require.NotNil(t, built.FinishedAt)
	require.ErrorIs(t, again, apperror.Conflict(""))
}

func Test_Built_SharesTheImageWithServicesBuiltFromIt(t *testing.T) {
	f := newFixture(t, false)
	rel, _ := f.usecase.Request(ctx, ana, "api", "main")
	report := entities.BuildReport{Digest: "sha256:" + digestHex, CommitSHA: sha, Tag: "r1", RunURL: "https://github.com/o/api/actions/runs/1"}

	_, err := f.usecase.Built(ctx, entities.BuildCaller{Repository: "o/api"}, rel.ID, report)

	require.NoError(t, err)
	shared := releasesOf(f.store, worker.ID)
	require.Len(t, shared, 1)
	require.Equal(t, entities.SourceShared, shared[0].Source)
	require.Equal(t, entities.StatusBuilt, shared[0].Status)
	require.Equal(t, "docker.io/o/api@sha256:"+digestHex, shared[0].Reference())
	require.Equal(t, sha, shared[0].CommitSHA)
	require.Equal(t, "main", shared[0].GitRef)
	require.Empty(t, releasesOf(f.store, web.ID))
	require.Equal(t, []string{"release.requested", "release.built", "release.shared"}, f.events.actions)
}

func Test_Built_RejectsMalformedReports(t *testing.T) {
	f := newFixture(t, false)
	rel, _ := f.usecase.Request(ctx, ana, "api", "main")

	_, err := f.usecase.Built(ctx, entities.BuildCaller{Repository: "o/api"}, rel.ID, entities.BuildReport{Digest: "latest"})

	require.ErrorIs(t, err, apperror.Invalid(""))
}

func Test_Failed_RecordsTheReason(t *testing.T) {
	f := newFixture(t, false)
	rel, _ := f.usecase.Request(ctx, ana, "api", "main")

	failed, err := f.usecase.Failed(ctx, entities.BuildCaller{Repository: "o/api"}, rel.ID, entities.BuildReport{Failure: "docker build exited 1"})

	require.NoError(t, err)
	require.Equal(t, entities.StatusFailed, failed.Status)
	require.Equal(t, "docker build exited 1", failed.Failure)
}

func Test_Register_ResolvesTheDigestAndIsBuiltAtOnce(t *testing.T) {
	f := newFixture(t, false)

	rel, err := f.usecase.Register(ctx, ana, "api", "11.2.0")
	_, missing := f.usecase.Register(ctx, ana, "api", "missing")
	_, noImage := f.usecase.Register(ctx, ana, "bare", "1.0")

	require.NoError(t, err)
	require.Equal(t, entities.StatusBuilt, rel.Status)
	require.Equal(t, entities.SourceRegistered, rel.Source)
	require.Equal(t, "11.2.0", rel.Tag)
	require.ErrorIs(t, missing, apperror.NotFound(""))
	require.ErrorIs(t, noImage, apperror.Invalid(""))
}

func Test_OnPush_BuildsOnlyAutoBuildServicesOnTheDefaultBranch(t *testing.T) {
	f := newFixture(t, false)
	push := entities.PushEvent{Repository: "o/api", Ref: "refs/heads/main", DefaultBranch: "main", After: sha}
	feature := push
	feature.Ref = "refs/heads/feature"

	started, err := f.usecase.OnPush(ctx, push)
	none, _ := f.usecase.OnPush(ctx, feature)

	require.NoError(t, err)
	require.Len(t, started, 1, "api and worker share an image, so it is built once")
	require.Len(t, f.builder.requests, 1)
	require.Equal(t, entities.TriggerPush, started[0].Trigger)
	require.Equal(t, "main", f.builder.requests[0].Ref, "GitHub only dispatches workflows on a branch or tag")
	require.Empty(t, none)
}

func Test_OnWorkflowRun_FailsTheReleaseNamedInTheRunTitle(t *testing.T) {
	f := newFixture(t, false)
	rel, _ := f.usecase.Request(ctx, ana, "api", "main")
	run := entities.WorkflowRunEvent{Repository: "o/api", Action: "completed", Conclusion: "cancelled", DisplayTitle: "platform release " + rel.ID, HTMLURL: "https://run"}

	require.NoError(t, f.usecase.OnWorkflowRun(ctx, entities.WorkflowRunEvent{Repository: "o/api", Action: "completed", Conclusion: "success", DisplayTitle: "platform release " + rel.ID}))
	require.Equal(t, entities.StatusBuilding, f.store.byID[rel.ID].Status)
	require.NoError(t, f.usecase.OnWorkflowRun(ctx, run))

	require.Equal(t, entities.StatusFailed, f.store.byID[rel.ID].Status)
	require.Equal(t, "workflow run cancelled", f.store.byID[rel.ID].Failure)
	require.Equal(t, "https://run", f.store.byID[rel.ID].RunURL)
}

func Test_Sweep_TimesOutOldUnfinishedBuilds(t *testing.T) {
	f := newFixture(t, false)
	clock.WithTime(time.Now().Add(-2 * time.Hour))
	old, _ := f.usecase.Request(ctx, ana, "api", "main")
	clock.WithTime(time.Now())
	fresh, _ := f.usecase.Request(ctx, ana, "api", "main")

	swept, err := f.usecase.Sweep(ctx)

	require.NoError(t, err)
	require.Equal(t, 1, swept)
	require.Equal(t, "timed out after 1h0m0s", f.store.byID[old.ID].Failure)
	require.Equal(t, entities.StatusBuilding, f.store.byID[fresh.ID].Status)
}

func releasesOf(s *store, serviceID string) []entities.Release {
	result := []entities.Release{}
	for _, rel := range s.byID {
		if rel.ServiceID == serviceID {
			result = append(result, rel)
		}
	}
	return result
}
