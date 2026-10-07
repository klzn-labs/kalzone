package requestctx

import (
	"github.com/adrianozp/platform/pkg/actor"
	"github.com/gin-gonic/gin"
)

const viewerKey = "platform.viewer"

type Viewer struct {
	Actor     actor.Actor
	Name      string
	AvatarURL string
	CSRFToken string
}

func SetViewer(c *gin.Context, v Viewer) {
	c.Set(viewerKey, v)
}

func ViewerFrom(c *gin.Context) Viewer {
	v, _ := c.Get(viewerKey)
	viewer, _ := v.(Viewer)
	return viewer
}

func ActorFrom(c *gin.Context) actor.Actor {
	return ViewerFrom(c).Actor
}
