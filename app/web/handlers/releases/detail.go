package releases

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	release "github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

var errNoSuchRelease = apperror.NotFound("release not found")

type EnvRun struct {
	Env    string
	Live   int
	Behind int
	Here   bool
}

type detail struct {
	data
	Release     release.Release
	Changelog   release.Changelog
	Groups      []PullGroup
	Runs        []EnvRun
	PromoteHint bool
}

func (h ReleasesHandler) Detail(c *gin.Context) {
	d, err := h.detail(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("number"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	title := d.Service.Name + " · r" + itoa(d.Release.Number)
	h.renderer.Render(c, http.StatusOK, "release", web.Page{Title: title, Nav: "services", Data: d})
}

func (h ReleasesHandler) detail(ctx context.Context, a actor.Actor, slug, number string) (detail, error) {
	frame, err := h.frame(ctx, a, slug)
	if err != nil {
		return detail{}, err
	}
	rel, err := h.numbered(ctx, slug, number)
	if err != nil {
		return detail{}, err
	}
	log, err := h.changelogs.Get(ctx, rel)
	if err != nil {
		return detail{}, err
	}
	d := detail{data: frame, Release: rel, Changelog: log, Groups: pullGroups(log.Commits)}
	d.Runs, err = h.runs(ctx, frame, rel)
	d.PromoteHint = promotable(d.Runs)
	return d, err
}

func (h ReleasesHandler) numbered(ctx context.Context, slug, number string) (release.Release, error) {
	if _, err := strconv.Atoi(number); err != nil {
		return release.Release{}, errNoSuchRelease
	}
	return h.releases.Resolve(ctx, slug, "r"+number)
}

func (h ReleasesHandler) runs(ctx context.Context, frame data, rel release.Release) ([]EnvRun, error) {
	envs := make([]catalogEnv, 0, len(frame.Envs))
	for _, tab := range frame.Envs {
		envs = append(envs, tab.Env)
	}
	chips, err := h.views.LiveReleases(ctx, frame.Service.Slug, envs)
	runs := make([]EnvRun, 0, len(chips))
	for _, chip := range chips {
		runs = append(runs, EnvRun{Env: chip.Slug, Live: chip.Release, Behind: rel.Number - chip.Release, Here: chip.Release == rel.Number})
	}
	return runs, err
}

func promotable(runs []EnvRun) bool {
	here := map[string]bool{}
	for _, r := range runs {
		here[r.Env] = r.Here
	}
	return here["dev"] && !here["prod"]
}
