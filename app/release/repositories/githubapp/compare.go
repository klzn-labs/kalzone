package githubapp

import (
	"context"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
)

type commitJSON struct {
	SHA    string `json:"sha"`
	Author *struct {
		Login string `json:"login"`
	} `json:"author"`
	Commit struct {
		Message string `json:"message"`
		Author  struct {
			Name string    `json:"name"`
			Date time.Time `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type compareJSON struct {
	Status       string       `json:"status"`
	AheadBy      int          `json:"ahead_by"`
	BehindBy     int          `json:"behind_by"`
	TotalCommits int          `json:"total_commits"`
	HTMLURL      string       `json:"html_url"`
	Commits      []commitJSON `json:"commits"`
}

type pullRefJSON struct {
	Number   int        `json:"number"`
	Title    string     `json:"title"`
	HTMLURL  string     `json:"html_url"`
	MergedAt *time.Time `json:"merged_at"`
}

func (c Client) Compare(ctx context.Context, repository, base, head string) (entities.Comparison, error) {
	var raw compareJSON
	path := "/repos/" + repository + "/compare/" + url.PathEscape(base) + "..." + url.PathEscape(head)
	if err := c.installed(ctx, repository, http.MethodGet, path, nil, http.StatusOK, &raw); err != nil {
		return entities.Comparison{}, err
	}
	return raw.toComparison(), nil
}

func (r compareJSON) toComparison() entities.Comparison {
	commits := make([]entities.Commit, 0, len(r.Commits))
	for _, c := range slices.Backward(r.Commits) {
		commits = append(commits, c.toCommit())
	}
	return entities.Comparison{
		Status: r.Status, AheadBy: r.AheadBy, BehindBy: r.BehindBy, Commits: commits,
		URL: r.HTMLURL, Truncated: r.TotalCommits > len(r.Commits),
	}
}

func (c commitJSON) toCommit() entities.Commit {
	author := c.Commit.Author.Name
	if c.Author != nil && c.Author.Login != "" {
		author = c.Author.Login
	}
	return entities.ParseCommit(c.SHA, c.Commit.Message, author, c.Commit.Author.Date.UTC())
}

func (c Client) Commit(ctx context.Context, repository, sha string) (entities.Commit, error) {
	var raw commitJSON
	err := c.installed(ctx, repository, http.MethodGet, "/repos/"+repository+"/commits/"+url.PathEscape(sha), nil, http.StatusOK, &raw)
	return raw.toCommit(), err
}

func (c Client) PullFor(ctx context.Context, repository, sha string) (*entities.PullRef, error) {
	var pulls []pullRefJSON
	path := "/repos/" + repository + "/commits/" + url.PathEscape(sha) + "/pulls"
	if err := c.installed(ctx, repository, http.MethodGet, path, nil, http.StatusOK, &pulls); err != nil {
		return nil, err
	}
	for _, p := range pulls {
		if p.MergedAt != nil {
			return &entities.PullRef{Number: p.Number, Title: p.Title, URL: p.HTMLURL}, nil
		}
	}
	return nil, nil
}
