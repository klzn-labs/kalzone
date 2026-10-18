package webhooks

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/internal/httpserver"
	"github.com/klzn-labs/kalzone/pkg/config"
)

const maxPayload = 5 << 20

type Builds interface {
	OnPush(ctx context.Context, push entities.PushEvent) ([]entities.Release, error)
	OnWorkflowRun(ctx context.Context, run entities.WorkflowRunEvent) error
}

type WebhooksHandler struct {
	secret []byte
	builds Builds
}

func NewWebhooksHandler(cfg config.Config, builds Builds) WebhooksHandler {
	return WebhooksHandler{secret: []byte(cfg.GitHub.WebhookSecret), builds: builds}
}

func (h WebhooksHandler) Receive(c *gin.Context) {
	if len(h.secret) == 0 {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxPayload))
	if err != nil || !h.signed(body, c.GetHeader("X-Hub-Signature-256")) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	if err := h.dispatch(c.Request.Context(), c.GetHeader("X-GitHub-Event"), body); err != nil {
		log.Printf("webhook %s: %v", c.GetHeader("X-GitHub-Event"), err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h WebhooksHandler) signed(body []byte, header string) bool {
	sent, found := strings.CutPrefix(header, "sha256=")
	if !found {
		return false
	}
	decoded, err := hex.DecodeString(sent)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, h.secret)
	mac.Write(body)
	return hmac.Equal(decoded, mac.Sum(nil))
}

func (h WebhooksHandler) dispatch(ctx context.Context, event string, body []byte) error {
	switch event {
	case "push":
		return h.push(ctx, body)
	case "workflow_run":
		return h.workflowRun(ctx, body)
	}
	return nil
}

type repository struct {
	FullName      string `json:"full_name"`
	DefaultBranch string `json:"default_branch"`
}

func (h WebhooksHandler) push(ctx context.Context, body []byte) error {
	var payload struct {
		Ref        string     `json:"ref"`
		After      string     `json:"after"`
		Repository repository `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	_, err := h.builds.OnPush(ctx, entities.PushEvent{
		Repository: payload.Repository.FullName, Ref: payload.Ref,
		DefaultBranch: payload.Repository.DefaultBranch, After: payload.After,
	})
	return err
}

func (h WebhooksHandler) workflowRun(ctx context.Context, body []byte) error {
	var payload struct {
		Action      string `json:"action"`
		WorkflowRun struct {
			Conclusion   string `json:"conclusion"`
			DisplayTitle string `json:"display_title"`
			HTMLURL      string `json:"html_url"`
		} `json:"workflow_run"`
		Repository repository `json:"repository"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return err
	}
	run := payload.WorkflowRun
	return h.builds.OnWorkflowRun(ctx, entities.WorkflowRunEvent{
		Repository: payload.Repository.FullName, Action: payload.Action, Conclusion: run.Conclusion,
		DisplayTitle: run.DisplayTitle, HTMLURL: run.HTMLURL,
	})
}

func RegisterWebhooksRoutes(public httpserver.Public, h WebhooksHandler) {
	public.POST("/webhooks/github", h.Receive)
}
