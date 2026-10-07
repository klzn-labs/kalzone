package entities

import (
	"regexp"
	"strings"

	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/slug"
)

var (
	githubRepoPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,38})/[A-Za-z0-9._-]{1,100}$`)
	imagePattern      = regexp.MustCompile(`^[a-z0-9]+(?:[._/:-][a-z0-9]+)*$`)
	workflowPattern   = regexp.MustCompile(`^[A-Za-z0-9._-]{1,90}\.ya?ml$`)
)

func (s Service) Validate() error {
	if err := slug.Validate("service", s.Slug); err != nil {
		return err
	}
	if err := slug.ValidateName("service", s.Name); err != nil {
		return err
	}
	if len(s.Description) > 2000 {
		return apperror.Invalid("description must be at most 2000 characters")
	}
	if err := validateSources(s.GitHubRepo, s.ImageRepository); err != nil {
		return err
	}
	return s.validateBuild()
}

func (s Service) validateBuild() error {
	if !workflowPattern.MatchString(s.BuildWorkflow) {
		return apperror.Invalid("build workflow must be a file name in .github/workflows, like platform-build.yml")
	}
	if s.AutoBuild && !s.CanBuild() {
		return apperror.Invalid("auto-build needs a GitHub repository and an image repository")
	}
	return nil
}

func (s Service) CanBuild() bool {
	return s.GitHubRepo != "" && s.ImageRepository != ""
}

func validateSources(githubRepo, image string) error {
	if githubRepo != "" && !githubRepoPattern.MatchString(githubRepo) {
		return apperror.Invalid("GitHub repository must look like owner/name")
	}
	if image != "" && (len(image) > 255 || !imagePattern.MatchString(image)) {
		return apperror.Invalid("image repository must be a lowercase reference without tag or digest, like docker.io/team/api")
	}
	return nil
}

func (c ServiceChanges) ApplyTo(s Service) Service {
	s.Name = valueOr(c.Name, s.Name)
	s.Description = valueOr(c.Description, s.Description)
	s.GitHubRepo = valueOr(c.GitHubRepo, s.GitHubRepo)
	s.ImageRepository = valueOr(c.ImageRepository, s.ImageRepository)
	s.BuildWorkflow = valueOr(c.BuildWorkflow, s.BuildWorkflow)
	if c.AutoBuild != nil {
		s.AutoBuild = *c.AutoBuild
	}
	return s
}

func valueOr(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return strings.TrimSpace(*value)
}
