package httpserver

import (
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/gin-gonic/gin"
)

type API struct {
	group   *gin.RouterGroup
	catalog *apispec.Catalog
}

func (a API) Handle(ops ...apispec.Operation) {
	for _, op := range ops {
		a.group.Handle(op.Method, op.Path, op.Handler)
		a.catalog.Add(op)
	}
}

type Web struct{ *gin.RouterGroup }

type Public struct{ *gin.RouterGroup }

type Guards struct {
	API     gin.HandlerFunc
	Web     gin.HandlerFunc
	Session gin.HandlerFunc
	Token   gin.HandlerFunc
}

type Session struct{ *gin.RouterGroup }

func NewRoutes(engine *gin.Engine, guards Guards, catalog *apispec.Catalog) (API, Web, Public, Session) {
	api := engine.Group(apispec.BasePath, guards.API)
	web := engine.Group("/", guards.Web)
	session := engine.Group("/", guards.Session)
	return API{group: api, catalog: catalog}, Web{web}, Public{&engine.RouterGroup}, Session{session}
}
