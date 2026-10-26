package queues

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/app/queue/usecases/queues"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

func (h QueuesHandler) Create(c *gin.Context) {
	settings, err := settingsFrom(c)
	if err != nil {
		web.Fail(c, err)
		return
	}
	q, err := h.queues.Create(c.Request.Context(), requestctx.ActorFrom(c), queues.Draft{
		Name: strings.TrimSpace(c.PostForm("name")), Environment: c.PostForm("environment"),
		Consumer: strings.TrimSpace(c.PostForm("consumer")), Settings: settings, Publishers: splitList(c.PostForm("publishers")),
	})
	h.finish(c, err, "/resources/queues/"+q.Environment+"/"+q.Name, "Queue "+q.Name+" created")
}

func (h QueuesHandler) Edit(c *gin.Context) {
	settings, err := settingsFrom(c)
	if err != nil {
		web.Fail(c, err)
		return
	}
	version, _ := strconv.Atoi(c.PostForm("version"))
	publishers := splitList(c.PostForm("publishers"))
	change := queues.Change{Settings: &settings, Publishers: &publishers, Message: c.PostForm("message"), BaseVersion: version}
	_, err = h.queues.Update(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), change)
	h.finish(c, err, h.base(c)+"?tab=settings", "Settings saved")
}

func (h QueuesHandler) Pause(c *gin.Context) {
	_, err := h.queues.SetPaused(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), true)
	h.finish(c, err, h.base(c), "Delivery paused; publishes are still accepted")
}

func (h QueuesHandler) Resume(c *gin.Context) {
	_, err := h.queues.SetPaused(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), false)
	h.finish(c, err, h.base(c), "Delivery resumed")
}

func (h QueuesHandler) Delete(c *gin.Context) {
	err := h.queues.Delete(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), c.PostForm("force") == "on")
	h.finish(c, err, "/resources?kind=queue", "Queue "+c.Param("name")+" deleted")
}

func (h QueuesHandler) Replay(c *gin.Context) {
	err := h.queues.Replay(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"), c.Param("id"))
	h.finish(c, err, h.base(c), "Replay requested; the gateway runs it within seconds")
}

func (h QueuesHandler) ReplayAll(c *gin.Context) {
	err := h.queues.ReplayAll(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	h.finish(c, err, h.base(c), "Replay of every dead letter requested")
}

func (h QueuesHandler) Purge(c *gin.Context) {
	err := h.queues.PurgeDead(c.Request.Context(), requestctx.ActorFrom(c), c.Param("env"), c.Param("name"))
	h.finish(c, err, h.base(c), "Purge of the dead letters requested")
}

func (h QueuesHandler) base(c *gin.Context) string {
	return "/resources/queues/" + c.Param("env") + "/" + c.Param("name")
}

func (h QueuesHandler) finish(c *gin.Context, err error, location, message string) {
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, location, message)
}

func splitList(raw string) []string {
	result := []string{}
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}

func settingsFrom(c *gin.Context) (entities.Settings, error) {
	numbers, err := formNumbers(c, "max_in_flight", "max_attempts", "max_message_bytes")
	if err != nil {
		return entities.Settings{}, err
	}
	rate, rateErr := strconv.ParseFloat(strings.TrimSpace(c.PostForm("rate_per_second")), 64)
	minimum, minErr := optionalRate(c.PostForm("min_rate_per_second"))
	durations, durErr := formDurations(c, "backoff_initial", "backoff_max", "delivery_timeout", "retention")
	if rateErr != nil || minErr != nil || durErr != nil {
		return entities.Settings{}, apperror.Invalid("rates must be numbers and durations look like 30s, 5m or 24h")
	}
	return entities.Settings{
		Path: strings.TrimSpace(c.PostForm("path")), RatePerSecond: rate, MaxInFlight: numbers["max_in_flight"],
		Adaptive: c.PostForm("adaptive") == "on", MinRatePerSecond: minimum,
		MaxAttempts: numbers["max_attempts"], MaxMessageBytes: numbers["max_message_bytes"],
		Backoff:         entities.Backoff{Kind: c.PostForm("backoff_kind"), Initial: durations["backoff_initial"], Max: durations["backoff_max"]},
		DeliveryTimeout: durations["delivery_timeout"], Retention: durations["retention"],
	}, nil
}

func optionalRate(raw string) (float64, error) {
	if raw = strings.TrimSpace(raw); raw == "" {
		return 0, nil
	}
	return strconv.ParseFloat(raw, 64)
}

func formNumbers(c *gin.Context, fields ...string) (map[string]int, error) {
	result := map[string]int{}
	for _, field := range fields {
		n, err := strconv.Atoi(strings.TrimSpace(c.PostForm(field)))
		if err != nil {
			return nil, apperror.Invalid(strings.ReplaceAll(field, "_", " ") + " must be a whole number")
		}
		result[field] = n
	}
	return result, nil
}

func formDurations(c *gin.Context, fields ...string) (map[string]time.Duration, error) {
	result := map[string]time.Duration{}
	for _, field := range fields {
		d, err := time.ParseDuration(strings.TrimSpace(c.PostForm(field)))
		if err != nil {
			return nil, err
		}
		result[field] = d
	}
	return result, nil
}
