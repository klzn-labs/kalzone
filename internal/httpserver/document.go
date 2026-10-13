package httpserver

import (
	"net/http"

	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/pkg/config"
	"github.com/gin-gonic/gin"
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
		raw, err := apispec.Build(api.catalog, cfg.BaseURL)
		doc.raw = raw
		return err
	}))
}

func (d *document) serve(c *gin.Context) {
	c.Data(http.StatusOK, "application/json", d.raw)
}
