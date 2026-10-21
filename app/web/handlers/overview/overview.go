package overview

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	approvals "github.com/klzn-labs/kalzone/app/approvals/entities"
	audit "github.com/klzn-labs/kalzone/app/audit/entities"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	deploy "github.com/klzn-labs/kalzone/app/deploy/entities"
	team "github.com/klzn-labs/kalzone/app/team/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/clock"
)

const (
	listed        = 3
	activityShown = 6
	healthyWithin = time.Minute
)

type Services interface {
	List(ctx context.Context, filter catalog.ServiceFilter) ([]catalog.Service, error)
}

type Teams interface {
	List(ctx context.Context) ([]team.Team, error)
}

type Events interface {
	List(ctx context.Context, filter audit.Filter) ([]audit.Event, error)
}

type Requests interface {
	List(ctx context.Context, a actor.Actor, status approvals.Status) ([]approvals.Request, error)
	Awaiting(ctx context.Context, a actor.Actor) ([]approvals.Request, error)
}

type Deployments interface {
	Live(ctx context.Context, serviceSlug, environment string) (deploy.Deployment, bool, error)
	FinishedFor(ctx context.Context, a actor.Actor, environment string, since time.Time) ([]deploy.Deployment, error)
}

type Agents interface {
	Agents(ctx context.Context) ([]deploy.Agent, error)
}

type Deps struct {
	Services    Services
	Teams       Teams
	Events      Events
	Requests    Requests
	Deployments Deployments
	Agents      Agents
}

type AgentState struct {
	Agent   deploy.Agent
	Healthy bool
}

type data struct {
	Waiting      []approvals.Request
	WaitingCount int
	Decided      []approvals.Request
	Activity     []web.Activity
	From         catalog.Environment
	To           catalog.Environment
	Lanes        []Lane
	Behind       int
	InStep       int
	Deploys      Chart
	Agents       []AgentState
	Environments []catalog.Environment
	Services     int
	Teams        int
}

type OverviewHandler struct {
	renderer *web.Renderer
	views    web.Views
	deps     Deps
}

func NewOverviewHandler(renderer *web.Renderer, views web.Views, deps Deps) OverviewHandler {
	return OverviewHandler{renderer: renderer, views: views, deps: deps}
}

func (h OverviewHandler) Show(c *gin.Context) {
	d, err := h.build(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	h.renderer.Render(c, http.StatusOK, "overview", web.Page{Title: "Overview", Nav: "overview", Data: d})
}

func (h OverviewHandler) build(ctx context.Context, a actor.Actor) (data, error) {
	d := data{}
	for _, step := range []func(context.Context, actor.Actor, *data) error{h.addRequests, h.addActivity, h.addLanes, h.addDeploys, h.addPlatform} {
		if err := step(ctx, a, &d); err != nil {
			return data{}, err
		}
	}
	return d, nil
}

func (h OverviewHandler) addRequests(ctx context.Context, a actor.Actor, d *data) error {
	waiting, err := h.deps.Requests.Awaiting(ctx, a)
	if err != nil {
		return err
	}
	all, err := h.deps.Requests.List(ctx, a, "")
	d.WaitingCount, d.Waiting, d.Decided = len(waiting), firstOf(waiting, listed), firstOf(decided(all), listed)
	return err
}

func decided(requests []approvals.Request) []approvals.Request {
	result := []approvals.Request{}
	for _, r := range requests {
		if !r.Pending() {
			result = append(result, r)
		}
	}
	return result
}

func firstOf[T any](items []T, n int) []T {
	return items[:min(n, len(items))]
}

func (h OverviewHandler) addActivity(ctx context.Context, _ actor.Actor, d *data) error {
	events, err := h.deps.Events.List(ctx, audit.Filter{Limit: activityShown})
	d.Activity = web.ActivityFrom(events)
	return err
}

func (h OverviewHandler) addLanes(ctx context.Context, a actor.Actor, d *data) error {
	envs, err := h.views.Environments(ctx)
	if err != nil {
		return err
	}
	d.Environments = envs
	from, to, ok := promotionPair(envs)
	if !ok {
		return nil
	}
	d.From, d.To = from, to
	services, err := h.visibleServices(ctx, a)
	if err != nil {
		return err
	}
	d.Services = len(services)
	d.Lanes, err = h.lanes(ctx, services, from.Slug, to.Slug)
	sortLanes(d.Lanes)
	d.Behind, d.InStep = waiting(d.Lanes), inStep(d.Lanes)
	return err
}

func (h OverviewHandler) visibleServices(ctx context.Context, a actor.Actor) ([]catalog.Service, error) {
	all, err := h.deps.Services.List(ctx, catalog.ServiceFilter{})
	visible := []catalog.Service{}
	for _, s := range all {
		if a.Sees(s.OwnerTeamID) {
			visible = append(visible, s)
		}
	}
	return visible, err
}

func (h OverviewHandler) lanes(ctx context.Context, services []catalog.Service, from, to string) ([]Lane, error) {
	teams, err := h.views.TeamsByID(ctx, ownerIDs(services))
	if err != nil {
		return nil, err
	}
	lanes := make([]Lane, 0, len(services))
	for _, s := range services {
		l := Lane{Service: s, Team: teams[s.OwnerTeamID]}
		if l.From, err = h.stage(ctx, s.Slug, from); err != nil {
			return nil, err
		}
		if l.To, err = h.stage(ctx, s.Slug, to); err != nil {
			return nil, err
		}
		lanes = append(lanes, l)
	}
	return lanes, nil
}

func (h OverviewHandler) stage(ctx context.Context, slug, env string) (Stage, error) {
	live, found, err := h.deps.Deployments.Live(ctx, slug, env)
	if err != nil || !found {
		return Stage{}, err
	}
	return Stage{Release: live.ReleaseNumber, At: live.FinishedAt}, nil
}

func ownerIDs(services []catalog.Service) []string {
	ids := make([]string, 0, len(services))
	for _, s := range services {
		ids = append(ids, s.OwnerTeamID)
	}
	return ids
}

func (h OverviewHandler) addDeploys(ctx context.Context, a actor.Actor, d *data) error {
	if d.To.Slug == "" {
		return nil
	}
	now := clock.Now()
	finished, err := h.deps.Deployments.FinishedFor(ctx, a, d.To.Slug, midnight(now).AddDate(0, 0, 1-chartDays))
	d.Deploys = chartOf(finished, now)
	return err
}

func (h OverviewHandler) addPlatform(ctx context.Context, _ actor.Actor, d *data) error {
	agents, err := h.deps.Agents.Agents(ctx)
	if err != nil {
		return err
	}
	for _, ag := range agents {
		d.Agents = append(d.Agents, AgentState{Agent: ag, Healthy: clock.Now().Sub(ag.LastSeenAt) < healthyWithin})
	}
	teams, err := h.deps.Teams.List(ctx)
	d.Teams = len(teams)
	return err
}

func RegisterOverviewRoutes(routes httpserver.Web, h OverviewHandler) {
	routes.GET("/", h.Show)
}
