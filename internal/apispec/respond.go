package apispec

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/internal/httperror"
	"github.com/klzn-labs/kalzone/pkg/page"
)

type ListParams struct {
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"20" description:"items per page"`
	Cursor string `query:"cursor" description:"next_cursor of the previous page"`
}

func PageRequest(c *gin.Context) (page.Request, error) {
	return page.Parse(c.Query("limit"), c.Query("cursor"))
}

func Respond(c *gin.Context, status int, body any, err error) {
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	c.JSON(status, body)
}

func RespondPage[E, D any](c *gin.Context, load func(page.Request) (page.Page[E], error), convert func(E) D) {
	p, err := PageRequest(c)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result, err := load(p)
	Respond(c, http.StatusOK, Items(result, convert), err)
}

func RespondWindow[E, D any](c *gin.Context, load func() ([]E, error), key func(E) string, convert func(E) D) {
	p, err := PageRequest(c)
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	all, err := load()
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result, err := Window(all, p, key, convert)
	Respond(c, http.StatusOK, result, err)
}
