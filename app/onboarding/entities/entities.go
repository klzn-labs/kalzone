package entities

import "strings"

const (
	StatusOK      = "ok"
	StatusFailed  = "failed"
	StatusSkipped = "skipped"

	CheckGitHubApp  = "github_app"
	CheckWorkflow   = "workflow"
	CheckDockerfile = "dockerfile"
	CheckRegistry   = "registry"
	CheckConfig     = "config"

	DefaultBuildWorkflow = "platform-build.yml"
	OnboardingBranch     = "platform-onboarding"
)

type Service struct {
	ID              string
	Slug            string
	OwnerTeamID     string
	Repository      string
	ImageRepository string
	BuildWorkflow   string
}

func (s Service) WorkflowPath() string {
	name := s.BuildWorkflow
	if name == "" {
		name = DefaultBuildWorkflow
	}
	return ".github/workflows/" + name
}

type Check struct {
	Name   string
	Status string
	Detail string
	Hint   string
}

type Report struct {
	Service string
	Ready   bool
	Checks  []Check
}

func NewReport(service string, checks []Check) Report {
	ready := true
	for _, c := range checks {
		ready = ready && c.Status == StatusOK
	}
	return Report{Service: service, Ready: ready, Checks: checks}
}

type DevSpec struct {
	Found      bool
	Port       int
	HealthPath string
}

type PullRequest struct {
	Branch  string
	Path    string
	Content string
	Message string
	Title   string
	Body    string
}

type OpenedPullRequest struct {
	URL    string
	Number int
	Branch string
}

const workflowTemplate = `name: Kalzone build
run-name: platform release ${{ inputs.release_id }}

on:
  workflow_dispatch:
    inputs:
      release_id:
        required: true
      image:
        required: true
      platform_url:
        required: true

permissions:
  contents: read
  id-token: write

jobs:
  build:
    uses: BUILD_WORKFLOW@main
    with:
      release_id: ${{ inputs.release_id }}
      image: ${{ inputs.image }}
      platform_url: ${{ inputs.platform_url }}
`

func Workflow(buildWorkflowRef string) string {
	return strings.Replace(workflowTemplate, "BUILD_WORKFLOW", buildWorkflowRef, 1)
}
