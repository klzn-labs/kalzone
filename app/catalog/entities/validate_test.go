package entities_test

import (
	"testing"

	"github.com/adrianozp/platform/app/catalog/entities"
	"github.com/stretchr/testify/require"
)

func Test_ServiceValidate(t *testing.T) {
	ok := entities.Service{Slug: "api", Name: "API", GitHubRepo: "adrianozp/go-plateful", ImageRepository: "docker.io/adrianozdp/plateful-api"}
	require.NoError(t, ok.Validate())

	broken := map[string]entities.Service{
		"slug":       {Slug: "API", Name: "API"},
		"name":       {Slug: "api", Name: " "},
		"repo":       {Slug: "api", Name: "API", GitHubRepo: "not a repo"},
		"image tag":  {Slug: "api", Name: "API", ImageRepository: "adrianozdp/api@sha256:abc"},
		"image case": {Slug: "api", Name: "API", ImageRepository: "AdrianoZdp/api"},
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
