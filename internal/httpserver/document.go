package httpserver

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/config"
	"go.uber.org/fx"
)

type document struct {
	raw []byte
}

func RegisterDocumentRoutes(lc fx.Lifecycle, api API, cfg config.Config) {
	doc := &document{}
	api.Handle(apispec.Operation{
		ID: "getOpenAPI", Method: http.MethodGet, Path: "/openapi.json", Summary: "This document: every operation, its scope, parameters and shapes",
		Status: http.StatusOK, Response: map[string]any{}, Handler: doc.serve,
	})
	lc.Append(fx.StartHook(func() error {
		raw, err := api.catalog.Document(cfg.BaseURL)
		doc.raw = raw
		return err
	}))
}

func (d *document) serve(c *gin.Context) {
	c.Data(http.StatusOK, "application/json", d.raw)
}
