package entities_test

import (
	"testing"

	"github.com/klzn-labs/kalzone/app/catalog/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/stretchr/testify/require"
)

func Test_ServiceValidate(t *testing.T) {
	ok := entities.Service{Slug: "api", Name: "API", GitHubRepo: "adrianozp/go-plateful", ImageRepository: "docker.io/adrianozdp/plateful-api", BuildWorkflow: "platform-build.yml", AutoBuild: true, AutoDeployDev: true}
	require.NoError(t, ok.Validate())

	wf := entities.DefaultBuildWorkflow
	broken := map[string]entities.Service{
		"slug":        {Slug: "API", Name: "API", BuildWorkflow: wf},
		"name":        {Slug: "api", Name: " ", BuildWorkflow: wf},
		"repo":        {Slug: "api", Name: "API", GitHubRepo: "not a repo", BuildWorkflow: wf},
		"image tag":   {Slug: "api", Name: "API", ImageRepository: "adrianozdp/api@sha256:abc", BuildWorkflow: wf},
		"image case":  {Slug: "api", Name: "API", ImageRepository: "AdrianoZdp/api", BuildWorkflow: wf},
		"workflow":    {Slug: "api", Name: "API", BuildWorkflow: "../evil.sh"},
		"auto build":  {Slug: "api", Name: "API", BuildWorkflow: wf, AutoBuild: true},
		"auto deploy": {Slug: "api", Name: "API", BuildWorkflow: wf, AutoDeployDev: true},
	}
	for name, s := range broken {
		require.Error(t, s.Validate(), name)
	}
}

func Test_ServiceChanges_ApplyOnlySetFields(t *testing.T) {
	name := "  New  "
	s := entities.ServiceChanges{Name: &name}.ApplyTo(entities.Service{Name: "Old", Description: "kept"})

	require.Equal(t, "New", s.Name)
	require.Equal(t, "kept", s.Description)
}

func Test_ServiceChanges_SetAndClearAutoDeployDev(t *testing.T) {
	on, off := true, false

	enabled := entities.ServiceChanges{AutoDeployDev: &on}.ApplyTo(entities.Service{AutoBuild: true})
	disabled := entities.ServiceChanges{AutoDeployDev: &off}.ApplyTo(enabled)
	untouched := entities.ServiceChanges{}.ApplyTo(enabled)

	require.True(t, enabled.AutoDeployDev)
	require.True(t, enabled.AutoBuild)
	require.False(t, disabled.AutoDeployDev)
	require.True(t, untouched.AutoDeployDev)
}

func Test_ServiceChanges_SetAndClearLimits(t *testing.T) {
	slow, unset := 800, 0
	before := entities.Service{Limits: entities.Limits{ResponseMS: 500, CPUPercent: 80}}

	after := entities.ServiceChanges{ResponseLimitMS: &slow, CPULimitPercent: &unset}.ApplyTo(before)

	require.Equal(t, entities.Limits{ResponseMS: 800}, after.Limits)
}

func Test_LimitsAreBounded(t *testing.T) {
	require.NoError(t, entities.Limits{}.Validate())
	require.NoError(t, entities.Limits{ResponseMS: 60000, CPUPercent: 100, DBCPUPercent: 400}.Validate())

	err := entities.Limits{CPUPercent: 101}.Validate()
	negative := entities.Limits{ResponseMS: -1}.Validate()

	require.ErrorIs(t, err, apperror.Invalid(""))
	require.Equal(t, "limits.cpu_percent", err.(*apperror.AppError).Field)
	require.Equal(t, "limits.response_ms", negative.(*apperror.AppError).Field)
}
