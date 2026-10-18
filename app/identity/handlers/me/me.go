package me

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/internal/requestctx"
)

type response struct {
	ID        string `json:"id"`
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
	Admin     bool   `json:"admin"`
	CSRFToken string `json:"csrf_token"`
}

func Me(c *gin.Context) {
	v := requestctx.ViewerFrom(c)
	c.JSON(http.StatusOK, response{ID: v.Actor.UserID, Login: v.Actor.Login, Name: v.Name, AvatarURL: v.AvatarURL, Admin: v.Actor.Admin, CSRFToken: v.CSRFToken})
}

func RegisterMeRoutes(api httpserver.API) {
	api.Handle(apispec.Operation{
		ID: "getMe", Method: http.MethodGet, Path: "/me", Summary: "The signed-in person and the CSRF token their writes need",
		Status: http.StatusOK, Response: response{}, Handler: Me,
	})
}
