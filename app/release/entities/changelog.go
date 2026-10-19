package entities

import (
	"errors"
	"slices"
	"strings"
	"time"
)

type ChangelogState string

const (
	ChangelogPending     ChangelogState = "pending"
	ChangelogAhead       ChangelogState = "ahead"
	ChangelogFirst       ChangelogState = "first"
	ChangelogDiverged    ChangelogState = "diverged"
	ChangelogUnavailable ChangelogState = "unavailable"
	ChangelogFailed      ChangelogState = "failed"
)

const (
	MaxChangelogAttempts = 5
	MaxPullLookups       = 30
	ErrorLimit           = 512
	FetchTimeout         = 2 * time.Minute
	retryFloor           = FetchTimeout + 30*time.Second
)

var ErrNoPullAccess = errors.New("the GitHub App cannot read pull requests on this repository")

var commitTypes = []string{"feat", "fix", "chore", "docs", "refactor", "perf", "test", "build", "ci", "style", "revert"}

type Changelog struct {
	ReleaseID     string
	BaseReleaseID string
	BaseNumber    int
	BaseSHA       string
	HeadSHA       string
	State         ChangelogState
	AheadBy       int
	BehindBy      int
	Truncated     bool
	CompareURL    string
	Commits       []Commit
	Attempts      int
	LastError     string
	UpdatedAt     time.Time
}

type Commit struct {
	SHA         string
	Title       string
	Type        string
	Author      string
	CommittedAt time.Time
	PullRequest *PullRef
}

type PullRef struct {
	Number int
	Title  string
	URL    string
}

func ParseCommit(sha, message, author string, at time.Time) Commit {
	title, _, _ := strings.Cut(message, "\n")
	title = strings.TrimSpace(title)
	return Commit{SHA: sha, Title: title, Type: commitType(title), Author: author, CommittedAt: at}
}

func commitType(title string) string {
	prefix, _, found := strings.Cut(title, ":")
	if !found {
		return "other"
	}
	prefix, _, _ = strings.Cut(prefix, "(")
	prefix = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(prefix), "!"))
	if slices.Contains(commitTypes, prefix) {
		return prefix
	}
	return "other"
}

func (c Changelog) RetryDue(now time.Time) bool {
	if c.State != ChangelogPending {
		return false
	}
	return now.Sub(c.UpdatedAt) >= max(time.Duration(1<<c.Attempts)*time.Minute, retryFloor)
}

type Comparison struct {
	Status    string
	AheadBy   int
	BehindBy  int
	Commits   []Commit
	URL       string
	Truncated bool
}
