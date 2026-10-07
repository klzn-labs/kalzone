package service

import (
	catalog "github.com/adrianozp/platform/app/catalog/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/gin-gonic/gin"
)

func (h ServiceHandler) Update(c *gin.Context) {
	changes := catalog.ServiceChanges{
		Name: formValue(c, "name"), Description: formValue(c, "description"),
		GitHubRepo: formValue(c, "github_repo"), ImageRepository: formValue(c, "image_repository"),
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
	if err := h.services.Delete(c.Request.Context(), requestctx.ActorFrom(c), slug); err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/services", "Service "+slug+" deleted")
}

func formValue(c *gin.Context, key string) *string {
	value, present := c.GetPostForm(key)
	if !present {
		return nil
	}
	return &value
}
