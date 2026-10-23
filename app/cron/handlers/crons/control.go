package crons

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/cron/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/internal/requestctx"
)

func (h CronsHandler) Pause(c *gin.Context) {
	cron, err := h.crons.SetPaused(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), true)
	h.respond(c, http.StatusOK, cron, err)
}

func (h CronsHandler) Resume(c *gin.Context) {
	cron, err := h.crons.SetPaused(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), false)
	h.respond(c, http.StatusOK, cron, err)
}

func (h CronsHandler) Run(c *gin.Context) {
	command, err := h.crons.Run(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	apispec.Respond(c, http.StatusAccepted, runRequestedDTO{RunID: command.ID, Note: runNote}, err)
}

func (h CronsHandler) Runs(c *gin.Context) {
	load := func() ([]entities.Run, error) {
		return h.crons.RunsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	}
	apispec.RespondWindow(c, load, runKey, toRunDTO)
}
