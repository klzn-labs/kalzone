package releases

import (
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
)

type ChangelogDTO struct {
	BaseRelease int         `json:"base_release,omitempty"`
	BaseSHA     string      `json:"base_sha,omitempty"`
	HeadSHA     string      `json:"head_sha,omitempty"`
	State       string      `json:"state" enum:"pending,ahead,first,diverged,unavailable,failed"`
	AheadBy     int         `json:"ahead_by"`
	BehindBy    int         `json:"behind_by"`
	Truncated   bool        `json:"truncated"`
	CompareURL  string      `json:"compare_url,omitempty"`
	Error       string      `json:"error,omitempty"`
	Commits     []CommitDTO `json:"commits"`
}

type CommitDTO struct {
	SHA         string          `json:"sha"`
	Title       string          `json:"title"`
	Type        string          `json:"type"`
	Author      string          `json:"author,omitempty"`
	CommittedAt *time.Time      `json:"committed_at,omitempty"`
	PullRequest *PullRequestDTO `json:"pull_request,omitempty"`
}

type PullRequestDTO struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
}

func toChangelogDTO(c entities.Changelog) *ChangelogDTO {
	commits := make([]CommitDTO, 0, len(c.Commits))
	for _, commit := range c.Commits {
		commits = append(commits, toCommitDTO(commit))
	}
	return &ChangelogDTO{
		BaseRelease: c.BaseNumber, BaseSHA: c.BaseSHA, HeadSHA: c.HeadSHA, State: string(c.State),
		AheadBy: c.AheadBy, BehindBy: c.BehindBy, Truncated: c.Truncated, CompareURL: c.CompareURL,
		Error: c.LastError, Commits: commits,
	}
}

func toCommitDTO(c entities.Commit) CommitDTO {
	dto := CommitDTO{SHA: c.SHA, Title: c.Title, Type: c.Type, Author: c.Author}
	if !c.CommittedAt.IsZero() {
		dto.CommittedAt = &c.CommittedAt
	}
	if c.PullRequest != nil {
		dto.PullRequest = &PullRequestDTO{Number: c.PullRequest.Number, Title: c.PullRequest.Title, URL: c.PullRequest.URL}
	}
	return dto
}
