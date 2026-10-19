package changelogs

import (
	"github.com/klzn-labs/kalzone/app/release/entities"
	"github.com/klzn-labs/kalzone/pkg/approval"
)

func shipCommit(c entities.Commit) approval.ShipCommit {
	s := approval.ShipCommit{SHA: c.SHA, Title: c.Title, Type: c.Type, Author: c.Author}
	if c.PullRequest != nil {
		s.PullNumber, s.PullTitle, s.PullURL = c.PullRequest.Number, c.PullRequest.Title, c.PullRequest.URL
	}
	return s
}

func shipCommits(commits []entities.Commit) []approval.ShipCommit {
	result := make([]approval.ShipCommit, 0, len(commits))
	for _, c := range commits {
		result = append(result, shipCommit(c))
	}
	return result
}

func flatten(releases []approval.ShipRelease) []approval.ShipCommit {
	result := []approval.ShipCommit{}
	for _, r := range releases {
		result = append(result, r.Commits...)
	}
	return result
}

func counts(commits []approval.ShipCommit) (int, int) {
	shas, pulls := map[string]bool{}, map[int]bool{}
	for _, c := range commits {
		shas[c.SHA] = true
		if c.PullNumber != 0 {
			pulls[c.PullNumber] = true
		}
	}
	return len(shas), len(pulls)
}
