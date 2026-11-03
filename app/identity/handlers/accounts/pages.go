package accounts

import (
	"context"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/identity/entities"
	"github.com/klzn-labs/kalzone/app/identity/handlers/auth"
	"github.com/klzn-labs/kalzone/app/identity/handlers/guard"
	usecase "github.com/klzn-labs/kalzone/app/identity/usecases/accounts"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const linkGone = "This link is no longer valid."

var joinTitles = map[string]string{
	string(entities.LinkSetup):  "Create the owner",
	string(entities.LinkInvite): "Join Kalzone",
	string(entities.LinkReset):  "Choose a new password",
}

type Setup interface {
	LinkKind(ctx context.Context, token string) (entities.LinkKind, error)
	Complete(ctx context.Context, token string, in usecase.SetupInput) (usecase.SetupResult, error)
}

type InviteAccepter interface {
	Accept(ctx context.Context, token, username, password, confirm string, signedIn bool) (entities.IssuedSession, error)
}

type PasswordResetter interface {
	UseReset(ctx context.Context, token, password, confirm, ip string) error
}

type PasswordSignIner interface {
	SignIn(ctx context.Context, username, password, ip string) (entities.IssuedSession, error)
}

type joinData struct {
	Kind     string
	Gone     string
	Failed   string
	Error    string
	Username string
	Bypass   bool
	SignedIn bool
	CSRF     string
	ResetOK  bool
}

type doneData struct {
	Token      string
	Command    string
	Unfinished bool
}

type PagesHandler struct {
	renderer *web.Renderer
	origin   gin.HandlerFunc
	github   bool
	secure   bool
	setup    Setup
	invites  InviteAccepter
	resets   PasswordResetter
	signIn   PasswordSignIner
	sessions guard.Authenticator
}

func NewPagesHandler(
	cfg config.Config, mode usecase.Mode, renderer *web.Renderer, setup Setup, invites InviteAccepter,
	resets PasswordResetter, signIn PasswordSignIner, sessions guard.Authenticator,
) PagesHandler {
	return PagesHandler{
		renderer: renderer, origin: SameOrigin(cfg.BaseURL, renderer), github: mode.GitHub, secure: cfg.SecureCookies(),
		setup: setup, invites: invites, resets: resets, signIn: signIn, sessions: sessions,
	}
}

func (h PagesHandler) Join(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, err := h.setup.LinkKind(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.gone(c, err)
		return
	}
	h.render(c, http.StatusOK, h.prefill(c, kind))
}

func (h PagesHandler) Submit(c *gin.Context) {
	if !h.available(c) {
		return
	}
	kind, err := h.setup.LinkKind(c.Request.Context(), c.Param("token"))
	if err != nil {
		h.gone(c, err)
		return
	}
	switch kind {
	case entities.LinkSetup:
		h.completeSetup(c)
	case entities.LinkInvite:
		h.acceptInvite(c)
	default:
		h.useReset(c)
	}
}

func (h PagesHandler) available(c *gin.Context) bool {
	if h.github {
		c.AbortWithStatus(http.StatusNotFound)
		return false
	}
	return true
}

func (h PagesHandler) prefill(c *gin.Context, kind entities.LinkKind) joinData {
	d := joinData{Kind: string(kind), Bypass: true}
	if kind == entities.LinkInvite {
		d.SignedIn, d.CSRF = h.signedIn(c)
	}
	return d
}

func (h PagesHandler) signedIn(c *gin.Context) (bool, string) {
	token, _ := c.Cookie(guard.SessionCookie)
	if token == "" {
		return false, ""
	}
	who, err := h.sessions.Authenticate(c.Request.Context(), token)
	if err != nil {
		return false, ""
	}
	return true, who.CSRFToken
}

func (h PagesHandler) completeSetup(c *gin.Context) {
	in := usecase.SetupInput{Username: c.PostForm("username"), Password: c.PostForm("password"), Confirm: c.PostForm("confirm"), Bypass: c.PostForm("bypass") == "on"}
	result, err := h.setup.Complete(c.Request.Context(), c.Param("token"), in)
	var unfinished usecase.SetupUnfinishedError
	if errors.As(err, &unfinished) {
		log.Printf("setup created the owner but did not finish: %v", unfinished.Err)
		h.renderer.Render(c, http.StatusOK, "setup_done", web.Page{Title: "Almost there", Data: doneData{Unfinished: true}})
		return
	}
	if err != nil {
		h.retry(c, joinData{Kind: string(entities.LinkSetup), Username: in.Username, Bypass: in.Bypass}, err)
		return
	}
	auth.SetSessionCookie(c, h.secure, result.Session)
	c.Header("Cache-Control", "no-store")
	h.renderer.Render(c, http.StatusOK, "setup_done", web.Page{Title: "You're in", Data: doneData{Token: result.AgentToken, Command: result.MCPCommand}})
}

func (h PagesHandler) acceptInvite(c *gin.Context) {
	d := h.prefill(c, entities.LinkInvite)
	d.Username = c.PostForm("username")
	issued, err := h.invites.Accept(c.Request.Context(), c.Param("token"), d.Username, c.PostForm("password"), c.PostForm("confirm"), d.SignedIn)
	if err != nil {
		h.retry(c, d, err)
		return
	}
	auth.SetSessionCookie(c, h.secure, issued)
	c.Redirect(http.StatusSeeOther, "/")
}

func (h PagesHandler) useReset(c *gin.Context) {
	d := joinData{Kind: string(entities.LinkReset)}
	err := h.resets.UseReset(c.Request.Context(), c.Param("token"), c.PostForm("password"), c.PostForm("confirm"), c.ClientIP())
	if err != nil {
		h.retry(c, d, err)
		return
	}
	d.ResetOK = true
	h.render(c, http.StatusOK, d)
}

func (h PagesHandler) retry(c *gin.Context, d joinData, err error) {
	if errors.Is(err, apperror.NotFound("")) {
		h.gone(c, err)
		return
	}
	status := http.StatusUnprocessableEntity
	if apperror.CodeOf(err) == apperror.CodeInternal {
		status = http.StatusInternalServerError
	}
	d.Error = web.UserMessage(err)
	h.render(c, status, d)
}

func (h PagesHandler) gone(c *gin.Context, err error) {
	if !errors.Is(err, apperror.NotFound("")) {
		h.render(c, http.StatusInternalServerError, joinData{Failed: web.UserMessage(err)})
		return
	}
	h.render(c, http.StatusNotFound, joinData{Gone: linkGone})
}

func (h PagesHandler) render(c *gin.Context, status int, d joinData) {
	title, ok := joinTitles[d.Kind]
	switch {
	case d.Failed != "":
		title = "Something went wrong"
	case !ok:
		title = "Link no longer valid"
	}
	h.renderer.Render(c, status, "join", web.Page{Title: title, Data: d})
}

func RegisterPagesRoutes(public httpserver.Public, h PagesHandler) {
	public.GET("/join/:token", h.Join)
	public.POST("/join/:token", h.origin, h.Submit)
	public.POST("/login", h.origin, h.SignInWithPassword)
}
