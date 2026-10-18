package releases

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const runTitlePrefix = "platform release "

var failedConclusions = map[string]bool{"failure": true, "cancelled": true, "timed_out": true, "startup_failure": true}

func (u ReleasesUsecase) OnPush(ctx context.Context, push entities.PushEvent) ([]entities.Release, error) {
	if !push.OnDefaultBranch() {
		return nil, nil
	}
	services, err := u.services.ServicesForRepository(ctx, push.Repository)
	if err != nil {
		return nil, err
	}
	started := []entities.Release{}
	for _, service := range oneAutoBuildPerImage(services) {
		rel, err := u.build(ctx, GitHubActor, service, push.DefaultBranch, entities.TriggerPush)
		if err != nil {
			log.Printf("auto-build of %s at %s did not start: %v", service.Slug, push.After, err)
		}
		started = append(started, rel)
	}
	return started, nil
}

func oneAutoBuildPerImage(services []entities.ServiceRef) []entities.ServiceRef {
	seen := map[string]bool{}
	result := []entities.ServiceRef{}
	for _, service := range services {
		if !service.AutoBuild || service.Decommissioning || seen[service.ImageRepository] {
			continue
		}
		seen[service.ImageRepository] = true
		result = append(result, service)
	}
	return result
}

func (u ReleasesUsecase) OnWorkflowRun(ctx context.Context, run entities.WorkflowRunEvent) error {
	releaseID, ours := strings.CutPrefix(run.DisplayTitle, runTitlePrefix)
	if !ours || run.Action != "completed" || !failedConclusions[run.Conclusion] {
		return nil
	}
	p, err := u.pendingFor(ctx, entities.BuildCaller{Repository: run.Repository}, strings.TrimSpace(releaseID))
	if err != nil {
		return nil
	}
	rel := p.release
	rel.RunURL = firstNonEmpty(rel.RunURL, truncate(run.HTMLURL, 500))
	_, err = u.fail(ctx, GitHubActor, p.service, rel, "workflow run "+strings.ReplaceAll(run.Conclusion, "_", " "))
	return err
}

func (u ReleasesUsecase) Sweep(ctx context.Context) (int, error) {
	stale, err := u.store.Unfinished(ctx, clock.Now().UTC().Add(-u.timeout))
	if err != nil {
		return 0, err
	}
	swept := 0
	for _, rel := range stale {
		if u.timeOut(ctx, rel) == nil {
			swept++
		}
	}
	return swept, nil
}

func (u ReleasesUsecase) timeOut(ctx context.Context, rel entities.Release) error {
	service, err := u.services.FindServiceByID(ctx, rel.ServiceID)
	if err != nil {
		return err
	}
	_, err = u.fail(ctx, GitHubActor, service, rel, "timed out after "+u.timeout.Round(time.Minute).String())
	return err
}
