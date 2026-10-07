package environment

import (
	"strconv"
	"strings"

	envconfig "github.com/adrianozp/platform/app/envconfig/entities"
	"github.com/adrianozp/platform/app/web"
	"github.com/adrianozp/platform/internal/requestctx"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/gin-gonic/gin"
)

func (h EnvironmentHandler) SaveSpec(c *gin.Context) {
	spec, err := specFrom(c)
	if err != nil {
		web.Fail(c, err)
		return
	}
	h.revise(c, envconfig.ChangeSet{Spec: &spec, Message: c.PostForm("message")}, "Runtime settings saved")
}

func (h EnvironmentHandler) SetVariable(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	h.revise(c, envconfig.ChangeSet{SetEnv: map[string]string{name: c.PostForm("value")}, Message: "set " + name}, name+" saved")
}

func (h EnvironmentHandler) RemoveVariable(c *gin.Context) {
	name := c.Param("name")
	h.revise(c, envconfig.ChangeSet{UnsetEnv: []string{name}, Message: "removed " + name}, name+" removed")
}

func (h EnvironmentHandler) SetSecret(c *gin.Context) {
	name := strings.TrimSpace(c.PostForm("name"))
	_, err := h.configs.SetSecret(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), name, c.PostForm("value"))
	h.finish(c, err, "Secret "+name+" saved")
}

func (h EnvironmentHandler) RemoveSecret(c *gin.Context) {
	name := c.Param("name")
	_, err := h.configs.UnsetSecret(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), name)
	h.finish(c, err, "Secret "+name+" removed")
}

func (h EnvironmentHandler) revise(c *gin.Context, cs envconfig.ChangeSet, message string) {
	_, err := h.configs.Revise(c.Request.Context(), requestctx.ActorFrom(c), c.Param("service"), c.Param("env"), cs)
	h.finish(c, err, message)
}

func (h EnvironmentHandler) finish(c *gin.Context, err error, message string) {
	if err != nil {
		web.Fail(c, err)
		return
	}
	web.Done(c, "/services/"+c.Param("service")+"/env/"+c.Param("env"), message)
}

func specFrom(c *gin.Context) (envconfig.Spec, error) {
	numbers := map[string]int{}
	for _, field := range []string{"port", "memory_mb", "cpu_millis", "replicas"} {
		value, err := strconv.Atoi(strings.TrimSpace(c.PostForm(field)))
		if err != nil {
			return envconfig.Spec{}, apperror.Invalid(strings.ReplaceAll(field, "_", " ") + " must be a whole number")
		}
		numbers[field] = value
	}
	return envconfig.Spec{
		Port: numbers["port"], HealthPath: strings.TrimSpace(c.PostForm("health_path")), Host: strings.ToLower(strings.TrimSpace(c.PostForm("host"))),
		MemoryMB: numbers["memory_mb"], CPUMillis: numbers["cpu_millis"], Replicas: numbers["replicas"],
		Command: strings.TrimSpace(c.PostForm("command")),
	}, nil
}
