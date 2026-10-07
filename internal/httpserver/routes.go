package httpserver

import "github.com/gin-gonic/gin"

type API struct{ *gin.RouterGroup }

type Web struct{ *gin.RouterGroup }

type Public struct{ *gin.RouterGroup }

type Guards struct {
	API     gin.HandlerFunc
	Web     gin.HandlerFunc
	Session gin.HandlerFunc
}

type Session struct{ *gin.RouterGroup }

func NewRoutes(engine *gin.Engine, guards Guards) (API, Web, Public, Session) {
	api := engine.Group("/api/v1", guards.API)
	web := engine.Group("/", guards.Web)
	session := engine.Group("/", guards.Session)
	return API{api}, Web{web}, Public{&engine.RouterGroup}, Session{session}
}
