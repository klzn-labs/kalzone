package service

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/app/web"
	"github.com/klzn-labs/kalzone/internal/requestctx"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

func (h ServiceHandler) Update(c *gin.Context) {
	changes := catalog.ServiceChanges{
		Name: formValue(c, "name"), Description: formValue(c, "description"),
		GitHubRepo: formValue(c, "github_repo"), ImageRepository: formValue(c, "image_repository"),
	}
	if err := limitChanges(c, &changes); err != nil {
		web.Fail(c, err)
		return
	}
	service, err := h.services.Update(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), changes)
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/services/"+service.Slug, "Saved")
}

func (h ServiceHandler) Transfer(c *gin.Context) {
	service, err := h.services.Transfer(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.PostForm("team"))
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/services/"+service.Slug, "Ownership moved")
}

func (h ServiceHandler) Delete(c *gin.Context) {
	slug := c.Param("service")
	if c.PostForm("confirm") != slug {
		web.Fail(c, errConfirm)
		return
	}
	result, err := h.decommission.Start(c.Request.Context(), requestctx.ActorFrom(c), slug)
	if err != nil {
		web.Fail(c, err)
		return
	}
	if result.Deleted {
		web.Done(c, "/services", "Service "+slug+" deleted")
		return
	}
	web.Done(c, "/services/"+slug, teardownNotice(slug, result.Steps))
}

func teardownNotice(slug string, steps []catalog.TeardownStep) string {
	parts := []string{}
	for _, s := range steps {
		parts = append(parts, s.Environment+" "+strings.ReplaceAll(string(s.Status), "_", " "))
	}
	return "Deleting " + slug + ": " + strings.Join(parts, ", ")
}

func formValue(c *gin.Context, key string) *string {
	value, present := c.GetPostForm(key)
	if !present {
		return nil
	}
	return &value
}

func limitChanges(c *gin.Context, changes *catalog.ServiceChanges) error {
	var err error
	if changes.ResponseLimitMS, err = formNumber(c, "limit_response_ms"); err != nil {
		return err
	}
	if changes.CPULimitPercent, err = formNumber(c, "limit_cpu_percent"); err != nil {
		return err
	}
	changes.DBCPULimitPercent, err = formNumber(c, "limit_db_cpu_percent")
	return err
}

func formNumber(c *gin.Context, key string) (*int, error) {
	raw, present := c.GetPostForm(key)
	if !present {
		return nil, nil
	}
	value := 0
	if trimmed := strings.TrimSpace(raw); trimmed != "" {
		parsed, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, apperror.Invalid(key + " must be a whole number, or empty for the platform default").WithField(key)
		}
		value = parsed
	}
	return &value, nil
}
