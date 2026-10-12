package queues

import (
	"net/http"

	"github.com/adrianozp/platform/app/queue/entities"
	"github.com/adrianozp/platform/internal/httperror"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/clock"
	"github.com/gin-gonic/gin"
)

func (h QueuesHandler) Stats(c *gin.Context) {
	st, found, err := h.queues.StatsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	if !found {
		c.JSON(http.StatusOK, gin.H{"stats": nil, "fresh": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"stats": st, "fresh": st.Fresh(clock.Now().UTC())})
}

func (h QueuesHandler) Versions(c *gin.Context) {
	list, err := h.queues.VersionsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	result := make([]versionDTO, 0, len(list))
	for _, v := range list {
		result = append(result, toVersionDTO(v))
	}
	c.JSON(http.StatusOK, gin.H{"versions": result})
}

func (h QueuesHandler) Dead(c *gin.Context) {
	st, _, err := h.queues.StatsFor(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	if err != nil {
		httperror.Abort(c, err)
		return
	}
	dead := st.NewestDead
	if dead == nil {
		dead = []entities.DeadLetter{}
	}
	c.JSON(http.StatusOK, gin.H{"dead": dead})
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
