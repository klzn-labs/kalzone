package entities

import (
	"regexp"
	"strings"

	"github.com/adrianozp/platform/pkg/apperror"
)

var (
	digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	commitPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)
	refPattern    = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)
	tagPattern    = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]{0,127}$`)
)

type BuildRequest struct {
	Repository string
	Workflow   string
	Ref        string
	Inputs     map[string]string
}

type BuildCaller struct {
	Repository string
}

type BuildReport struct {
	Digest    string
	CommitSHA string
	Tag       string
	RunURL    string
	Failure   string
}

type PushEvent struct {
	Repository    string
	Ref           string
	DefaultBranch string
	After         string
}

func (p PushEvent) OnDefaultBranch() bool {
	return p.Ref == "refs/heads/"+p.DefaultBranch && commitPattern.MatchString(p.After) && strings.Trim(p.After, "0") != ""
}

type WorkflowRunEvent struct {
	Repository   string
	Action       string
	Conclusion   string
	DisplayTitle string
	HTMLURL      string
}

func ValidateRef(ref string) error {
	if !refPattern.MatchString(ref) || strings.Contains(ref, "..") {
		return apperror.Invalid("ref must be a branch, tag or commit")
	}
	return nil
}

func ValidateTag(tag string) error {
	if !tagPattern.MatchString(tag) {
		return apperror.Invalid("tag must be a valid image tag, like 11.2.0")
	}
	return nil
}

func (b BuildReport) Validate() error {
	if !digestPattern.MatchString(b.Digest) {
		return apperror.Invalid("digest must look like sha256:<64 hex>")
	}
	if b.CommitSHA != "" && !commitPattern.MatchString(b.CommitSHA) {
		return apperror.Invalid("commit must be a full 40-character SHA")
	}
	return nil
}
