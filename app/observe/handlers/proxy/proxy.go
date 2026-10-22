package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const Prefix = "/grafana"

type GrafanaProxy struct {
	target *url.URL
}

func NewGrafanaProxy(cfg config.Config) (GrafanaProxy, error) {
	if cfg.Grafana.URL == "" {
		return GrafanaProxy{}, nil
	}
	target, err := url.Parse(cfg.Grafana.URL)
	return GrafanaProxy{target: target}, err
}

func (p GrafanaProxy) Serve(c *gin.Context) {
	viewer := requestctx.ViewerFrom(c)
	if p.target == nil || !viewer.Actor.Admin {
		c.String(http.StatusNotFound, "observability is not connected")
		return
	}
	proxy := &httputil.ReverseProxy{Rewrite: func(r *httputil.ProxyRequest) {
		r.Out.URL.Scheme, r.Out.URL.Host, r.Out.Host = p.target.Scheme, p.target.Host, p.target.Host
		identify(r.Out.Header, viewer)
	}}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func identify(h http.Header, viewer requestctx.Viewer) {
	for name := range h {
		if strings.HasPrefix(strings.ToLower(name), "x-webauth-") {
			h.Del(name)
		}
	}
	role := "Viewer"
	if viewer.Actor.Admin {
		role = "Admin"
	}
	h.Set("X-WEBAUTH-USER", viewer.Actor.Login)
	h.Set("X-WEBAUTH-NAME", viewer.Name)
	h.Set("X-WEBAUTH-ROLE", role)
}

func RegisterProxyRoutes(session httpserver.Session, p GrafanaProxy) {
	session.Any(Prefix+"/*path", p.Serve)
}
