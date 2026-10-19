package mysql

import (
	"time"

	"github.com/klzn-labs/kalzone/app/release/entities"
)

type release struct {
	ID          string `gorm:"primaryKey"`
	ServiceID   string
	Number      int
	Source      string
	GitRef      string
	CommitSHA   string
	Image       string
	Tag         string
	Digest      string
	Status      string
	Trigger     string `gorm:"column:trigger"`
	RequestedBy string
	RunURL      string `gorm:"column:run_url"`
	Failure     string
	CreatedAt   time.Time
	FinishedAt  *time.Time
}

func (release) TableName() string { return "releases" }

func fromRelease(r entities.Release) release {
	return release{
		ID: r.ID, ServiceID: r.ServiceID, Number: r.Number, Source: string(r.Source), GitRef: r.GitRef,
		CommitSHA: r.CommitSHA, Image: r.Image, Tag: r.Tag, Digest: r.Digest, Status: string(r.Status),
		Trigger: string(r.Trigger), RequestedBy: r.RequestedBy, RunURL: r.RunURL, Failure: r.Failure,
		CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt,
	}
}

func (m release) toEntity() entities.Release {
	return entities.Release{
		ID: m.ID, ServiceID: m.ServiceID, Number: m.Number, Source: entities.Source(m.Source), GitRef: m.GitRef,
		CommitSHA: m.CommitSHA, Image: m.Image, Tag: m.Tag, Digest: m.Digest, Status: entities.Status(m.Status),
		Trigger: entities.Trigger(m.Trigger), RequestedBy: m.RequestedBy, RunURL: m.RunURL, Failure: m.Failure,
		CreatedAt: m.CreatedAt, FinishedAt: m.FinishedAt,
	}
}

type changelog struct {
	ReleaseID     string `gorm:"primaryKey"`
	BaseReleaseID string
	BaseSHA       string `gorm:"column:base_sha"`
	HeadSHA       string `gorm:"column:head_sha"`
	State         string
	AheadBy       int
	BehindBy      int
	Truncated     bool
	CompareURL    string            `gorm:"column:compare_url"`
	Commits       []entities.Commit `gorm:"serializer:json"`
	Attempts      int
	LastError     string
	UpdatedAt     time.Time `gorm:"autoUpdateTime:false"`
}

func (changelog) TableName() string { return "release_changelogs" }

func fromChangelog(c entities.Changelog) changelog {
	return changelog{
		ReleaseID: c.ReleaseID, BaseReleaseID: c.BaseReleaseID, BaseSHA: c.BaseSHA, HeadSHA: c.HeadSHA, State: string(c.State),
		AheadBy: c.AheadBy, BehindBy: c.BehindBy, Truncated: c.Truncated, CompareURL: c.CompareURL,
		Commits: nonNil(c.Commits), Attempts: c.Attempts, LastError: c.LastError, UpdatedAt: c.UpdatedAt,
	}
}

func (m changelog) toEntity() entities.Changelog {
	return entities.Changelog{
		ReleaseID: m.ReleaseID, BaseReleaseID: m.BaseReleaseID, BaseSHA: m.BaseSHA, HeadSHA: m.HeadSHA,
		State: entities.ChangelogState(m.State), AheadBy: m.AheadBy, BehindBy: m.BehindBy, Truncated: m.Truncated,
		CompareURL: m.CompareURL, Commits: nonNil(m.Commits), Attempts: m.Attempts, LastError: m.LastError, UpdatedAt: m.UpdatedAt.UTC(),
	}
}

func nonNil(commits []entities.Commit) []entities.Commit {
	if commits == nil {
		return []entities.Commit{}
	}
	return commits
}
