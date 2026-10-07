package entities_test

import (
	"testing"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/stretchr/testify/require"
)

func Test_ServiceValidate(t *testing.T) {
	ok := entities.Service{Slug: "api", Name: "API", GitHubRepo: "adrianozp/go-plateful", ImageRepository: "docker.io/adrianozdp/plateful-api", BuildWorkflow: "platform-build.yml", AutoBuild: true}
	require.NoError(t, ok.Validate())

	wf := entities.DefaultBuildWorkflow
	broken := map[string]entities.Service{
		"slug":       {Slug: "API", Name: "API", BuildWorkflow: wf},
		"name":       {Slug: "api", Name: " ", BuildWorkflow: wf},
		"repo":       {Slug: "api", Name: "API", GitHubRepo: "not a repo", BuildWorkflow: wf},
		"image tag":  {Slug: "api", Name: "API", ImageRepository: "adrianozdp/api@sha256:abc", BuildWorkflow: wf},
		"image case": {Slug: "api", Name: "API", ImageRepository: "AdrianoZdp/api", BuildWorkflow: wf},
		"workflow":   {Slug: "api", Name: "API", BuildWorkflow: "../evil.sh"},
		"auto build": {Slug: "api", Name: "API", BuildWorkflow: wf, AutoBuild: true},
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
