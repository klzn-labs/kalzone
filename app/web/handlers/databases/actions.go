package databases

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/database/usecases/databases"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const nextDeployNote = "; it takes effect on the service's next deploy"

func (h DatabasesHandler) Create(c *gin.Context) {
	connections, err := connectionsFrom(c.PostForm("max_connections"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	db, err := h.databases.Create(c.Request.Context(), requestctx.ActorFrom(c), databases.Draft{
		Name: strings.TrimSpace(c.PostForm("name")), Environment: c.PostForm("environment"),
		Team: c.PostForm("team"), MaxConnections: connections,
	})
	h.finish(c, err, "/resources/databases/"+db.Environment+"/"+db.Name, "Database "+db.Name+" created")
}

func (h DatabasesHandler) Attach(c *gin.Context) {
	service := strings.TrimSpace(c.PostForm("service"))
	_, err := h.databases.Attach(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), service)
	h.finish(c, err, h.base(c), service+" attached"+nextDeployNote)
}

func (h DatabasesHandler) Detach(c *gin.Context) {
	_, err := h.databases.Detach(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), c.Param("service"))
	h.finish(c, err, h.base(c), c.Param("service")+" detached"+nextDeployNote)
}

var errConfirm = apperror.Invalid("type the database name to confirm")

func (h DatabasesHandler) Delete(c *gin.Context) {
	if c.PostForm("confirm") != c.Param("name") {
		web.Fail(c, errConfirm)
		return
	}
	err := h.databases.Delete(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	h.finish(c, err, "/resources?kind=database", "Database "+c.Param("name")+" deleted")
}

func (h DatabasesHandler) base(c *gin.Context) string {
	return "/resources/databases/" + c.Param("env") + "/" + c.Param("name")
}

func (h DatabasesHandler) finish(c *gin.Context, err error, location, message string) {
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, location, message)
}

func connectionsFrom(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, apperror.Invalid("max connections must be a whole number")
	}
	return n, nil
}
