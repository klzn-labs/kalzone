package queues

import (
	"net/http"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/internal/apispec"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/adrianozp/platform/pkg/page"
	"github.com/gin-gonic/gin"
)

func (h QueuesHandler) Stats(c *gin.Context) {
	st, _, err := h.queues.StatsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	apispec.Respond(c, http.StatusOK, toStatsDTO(st, st.Fresh(clock.Now().UTC())), err)
}

func (h QueuesHandler) Versions(c *gin.Context) {
	load := func(p page.Request) (page.Page[entities.Version], error) {
		return h.queues.VersionPageFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), p)
	}
	apispec.RespondPage(c, load, toVersionDTO)
}

func (h QueuesHandler) Dead(c *gin.Context) {
	load := func() ([]entities.DeadLetter, error) {
		st, _, err := h.queues.StatsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
		return st.NewestDead, err
	}
	apispec.RespondWindow(c, load, deadKey, func(d entities.DeadLetter) entities.DeadLetter { return d })
}

func deadKey(d entities.DeadLetter) string {
	return d.ID
}

func (h QueuesHandler) Replay(c *gin.Context) {
	err := h.queues.Replay(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), c.Param("id"))
	noContent(c, http.StatusAccepted, err)
}

func (h QueuesHandler) ReplayAll(c *gin.Context) {
	err := h.queues.ReplayAll(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	noContent(c, http.StatusAccepted, err)
}

func (h QueuesHandler) PurgeDead(c *gin.Context) {
	err := h.queues.PurgeDead(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	noContent(c, http.StatusAccepted, err)
}
