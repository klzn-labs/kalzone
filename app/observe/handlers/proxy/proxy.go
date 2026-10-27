package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/clock"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const (
	Prefix   = "/grafana"
	KeysPath = "/internal/grafana/keys.json"
)

type GrafanaProxy struct {
	target *url.URL
	signer Signer
}

func NewGrafanaProxy(cfg config.Config, signer Signer) (GrafanaProxy, error) {
	if cfg.Grafana.URL == "" {
		return GrafanaProxy{signer: signer}, nil
	}
	target, err := url.Parse(cfg.Grafana.URL)
	return GrafanaProxy{target: target, signer: signer}, err
}

func (p GrafanaProxy) Serve(c *gin.Context) {
	viewer := requestctx.ViewerFrom(c)
	if p.target == nil {
		c.String(http.StatusNotFound, "observability is not connected")
		return
	}
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.Out.URL.Scheme, r.Out.URL.Host, r.Out.Host = p.target.Scheme, p.target.Host, p.target.Host
		p.identify(r.Out.Header, viewer)
	}}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func (p GrafanaProxy) identify(h http.Header, viewer requestctx.Viewer) {
	for name := range h {
		if forwarded(name) {
			h.Del(name)
		}
	}
	h.Set("X-WEBAUTH-USER", viewer.Actor.Login)
	h.Set("X-WEBAUTH-NAME", viewer.Name)
	if !p.signer.Enabled() {
		return
	}
	if assertion, err := p.signer.Assertion(viewer.Actor.Login, viewer.Name, clock.Now()); err == nil {
		h.Set(AssertionHeader, assertion)
	}
}

func forwarded(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "x-webauth-") || lower == strings.ToLower(AssertionHeader)
}

func (p GrafanaProxy) Keys(c *gin.Context) {
	if !p.signer.Enabled() {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, p.signer.KeySet())
}

func RegisterProxyRoutes(session httpserver.Session, public httpserver.Public, p GrafanaProxy) {
	session.Any(Prefix+"/*path", p.Serve)
	public.GET(KeysPath, p.Keys)
}
