package httpserver

import "github.com/gin-gonic/gin"

type API struct{ *gin.RouterGroup }

type Web struct{ *gin.RouterGroup }

type Public struct{ *gin.RouterGroup }

type Guards struct {
	API gin.HandlerFunc
	Web gin.HandlerFunc
}

func NewRoutes(engine *gin.Engine, guards Guards) (API, Web, Public) {
	api := engine.Group("/api/v1", guards.API)
	web := engine.Group("/", guards.Web)
	return API{api}, Web{web}, Public{&engine.RouterGroup}
}
