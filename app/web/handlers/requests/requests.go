package requests

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/approvals/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

type Requests interface {
	List(ctx context.Context, a actor.Actor, status entities.Status) ([]entities.Request, error)
	Get(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	Awaiting(ctx context.Context, a actor.Actor) ([]entities.Request, error)
	MayDecide(ctx context.Context, a actor.Actor, r entities.Request) bool
	Preview(ctx context.Context, r entities.Request) (approval.Preview, error)
	Approve(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
	Reject(ctx context.Context, a actor.Actor, id, reason string) (entities.Request, error)
	Cancel(ctx context.Context, a actor.Actor, id string) (entities.Request, error)
}

type listView struct {
	Pending []entities.Request
	Recent  []entities.Request
}

type detailView struct {
	Request      entities.Request
	Preview      approval.Preview
	PreviewError string
	CanDecide    bool
	CanCancel    bool
}

type RequestsHandler struct {
	renderer *web.Renderer
	requests Requests
}

func NewRequestsHandler(renderer *web.Renderer, requests Requests) RequestsHandler {
	return RequestsHandler{renderer: renderer, requests: requests}
}

func (h RequestsHandler) List(c *gin.Context) {
	all, err := h.requests.List(c.Request.Context(), requestctx.ActorFrom(c), "")
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	view := listView{}
	for _, r := range all {
		if r.Pending() {
			view.Pending = append(view.Pending, r)
			continue
		}
		view.Recent = append(view.Recent, r)
	}
	h.renderer.Render(c, http.StatusOK, "requests", web.Page{Title: "Requests", Nav: "requests", Data: view})
}

func (h RequestsHandler) Show(c *gin.Context) {
	ctx, a := c.Request.Context(), requestctx.ActorFrom(c)
	r, err := h.requests.Get(ctx, a, c.Param("id"))
	if err != nil {
		web.ErrorPage(c, h.renderer, err)
		return
	}
	view := detailView{Request: r, CanDecide: r.Pending() && h.requests.MayDecide(ctx, a, r), CanCancel: r.Pending() && r.RequestedBy == a.UserID}
	view.Preview, err = h.requests.Preview(ctx, r)
	if err != nil && r.Pending() {
		view.PreviewError = web.UserMessage(err)
	}
	h.renderer.Render(c, http.StatusOK, "request", web.Page{Title: r.Summary, Nav: "requests", Data: view})
}

func (h RequestsHandler) Badge(c *gin.Context) {
	waiting, err := h.requests.Awaiting(c.Request.Context(), requestctx.ActorFrom(c))
	if err != nil || len(waiting) == 0 {
		c.Data(http.StatusOK, "text/html; charset=utf-8", nil)
		return
	}
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(fmt.Sprintf(`<span class="badge badge-warn">%d</span>`, len(waiting))))
}

func (h RequestsHandler) Approve(c *gin.Context) {
	r, err := h.requests.Approve(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	if r.Status == entities.StatusFailed {
		web.Done(c, r.ApprovePath(), "Approved, but it failed: "+r.ErrorMessage)
		return
	}
	web.Done(c, r.ApprovePath(), "Approved and done: "+r.Result)
}

func (h RequestsHandler) Reject(c *gin.Context) {
	r, err := h.requests.Reject(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"), c.PostForm("reason"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, r.ApprovePath(), "Rejected")
}

func (h RequestsHandler) Cancel(c *gin.Context) {
	r, err := h.requests.Cancel(c.Request.Context(), requestctx.ActorFrom(c), c.Param("id"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, r.ApprovePath(), "Cancelled")
}

func RegisterRequestsRoutes(routes httpserver.Web, h RequestsHandler) {
	routes.GET("/requests", h.List)
	routes.GET("/requests/badge", h.Badge)
	routes.GET("/requests/:id", h.Show)
	routes.POST("/requests/:id/approve", h.Approve)
	routes.POST("/requests/:id/reject", h.Reject)
	routes.POST("/requests/:id/cancel", h.Cancel)
}
