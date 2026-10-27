package crons

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	cronsUsecase "github.com/klzn-labs/kalzone/app/cron/usecases/crons"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

func (h CronsHandler) Create(c *gin.Context) {
	timeout, err := timeoutFrom(c.PostForm("timeout"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	cron, err := h.crons.Create(c.Request.Context(), requestctx.ActorFrom(c), cronsUsecase.Draft{
		Name: strings.TrimSpace(c.PostForm("name")), Environment: c.PostForm("environment"), Service: c.PostForm("service"),
		Schedule: strings.TrimSpace(c.PostForm("schedule")), Path: strings.TrimSpace(c.PostForm("path")),
		Payload: payloadFrom(c.PostForm("payload")), Timeout: timeout,
	})
	h.finish(c, err, "/resources/crons/"+cron.Environment+"/"+cron.Name, "Cron "+cron.Name+" created")
}

func (h CronsHandler) Edit(c *gin.Context) {
	change, err := changeFrom(c)
	if err != nil {
		web.Fail(c, err)
		return
	}
	_, err = h.crons.Update(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), change)
	h.finish(c, err, h.base(c), "Settings saved")
}

func (h CronsHandler) Pause(c *gin.Context) {
	_, err := h.crons.SetPaused(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), true)
	h.finish(c, err, h.base(c), "Cron paused; Run now still works")
}

func (h CronsHandler) Resume(c *gin.Context) {
	_, err := h.crons.SetPaused(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), false)
	h.finish(c, err, h.base(c), "Cron resumed")
}

func (h CronsHandler) RunNow(c *gin.Context) {
	_, err := h.crons.Run(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	h.finish(c, err, h.base(c), "Run requested; the gateway starts it within a few seconds")
}

func (h CronsHandler) Delete(c *gin.Context) {
	err := h.crons.Delete(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	h.finish(c, err, "/resources/crons", "Cron "+c.Param("name")+" deleted")
}

func (h CronsHandler) base(c *gin.Context) string {
	return "/resources/crons/" + c.Param("env") + "/" + c.Param("name")
}

func (h CronsHandler) finish(c *gin.Context, err error, location, message string) {
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, location, message)
}

func changeFrom(c *gin.Context) (cronsUsecase.Change, error) {
	timeout, err := timeoutFrom(c.PostForm("timeout"))
	if err != nil {
		return cronsUsecase.Change{}, err
	}
	schedule, path := strings.TrimSpace(c.PostForm("schedule")), strings.TrimSpace(c.PostForm("path"))
	payload := payloadFrom(c.PostForm("payload"))
	if payload == nil {
		payload = []byte("{}")
	}
	return cronsUsecase.Change{Schedule: &schedule, Path: &path, Payload: &payload, Timeout: &timeout}, nil
}

func timeoutFrom(raw string) (time.Duration, error) {
	if raw = strings.TrimSpace(raw); raw == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(raw)
	if err != nil {
		return 0, apperror.Invalid("timeout must be a duration such as 30s or 5m")
	}
	return timeout, nil
}

func payloadFrom(raw string) []byte {
	if raw = strings.TrimSpace(raw); raw == "" {
		return nil
	}
	return []byte(raw)
}
