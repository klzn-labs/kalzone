package web

import (
	"errors"
	"html/template"
	"log"
	"net/http"

	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/gin-gonic/gin"
)

func IsHTMX(c *gin.Context) bool {
	return c.GetHeader("HX-Request") == "true"
}

func Done(c *gin.Context, location, message string) {
	SetFlash(c, Flash{Message: message})
	if IsHTMX(c) {
		c.Header("HX-Location", location)
		c.Status(http.StatusNoContent)
		return
	}
	c.Redirect(http.StatusSeeOther, location)
}

func Fail(c *gin.Context, err error) {
	message := UserMessage(err)
	if IsHTMX(c) {
		c.Data(http.StatusUnprocessableEntity, "text/html; charset=utf-8", []byte(alertHTML(message)))
		return
	}
	SetFlash(c, Flash{Message: message, Error: true})
	c.Redirect(http.StatusSeeOther, backTo(c))
}

func UserMessage(err error) string {
	var appErr *apperror.AppError
	if errors.As(err, &appErr) && appErr.Code != apperror.CodeInternal {
		return appErr.Message
	}
	log.Printf("web action failed: %v", err)
	return "Something went wrong on our side. Try again in a moment."
}

func alertHTML(message string) string {
	return `<div class="alert alert-danger" role="alert">` + string(icon("alert")) + `<span>` + template.HTMLEscapeString(message) + `</span></div>`
}

func backTo(c *gin.Context) string {
	referer := c.GetHeader("Referer")
	if referer == "" {
		return "/"
	}
	return referer
}

func ErrorPage(c *gin.Context, r *Renderer, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, apperror.NotFound("")) {
		status = http.StatusNotFound
	}
	r.Render(c, status, "error", Page{Title: "Not found", Data: UserMessage(err)})
}
