package releases

import (
	"strings"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	release "github.com/klzn-labs/kalzone/app/release/entities"
)

const dispatchFailed = "dispatch failed"

type Wiring struct {
	Tone  string
	Label string
}

func wiringOf(service catalog.Service, list []release.Release) Wiring {
	if service.GitHubRepo == "" {
		return Wiring{Tone: "warn", Label: "repository not linked"}
	}
	for _, r := range list {
		if r.Source == release.SourceBuild && r.Final() {
			return wiringFrom(r)
		}
	}
	return Wiring{Label: "no builds yet"}
}

func wiringFrom(r release.Release) Wiring {
	if strings.HasPrefix(r.Failure, dispatchFailed) {
		return Wiring{Tone: "danger", Label: "last build couldn't start"}
	}
	return Wiring{Tone: "ok", Label: "workflow wired up"}
}

func failureSummary(failure string) string {
	switch {
	case missingWorkflow(failure):
		return "Build workflow not found"
	case strings.HasPrefix(failure, dispatchFailed):
		return "GitHub refused to start the build"
	}
	return failure
}

func missingWorkflow(failure string) bool {
	return strings.HasPrefix(failure, dispatchFailed) && strings.Contains(failure, "workflow was not found")
}
