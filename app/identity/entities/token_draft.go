package entities

import (
	"fmt"
	"regexp"
	"time"

	"github.com/adrianozp/platform/pkg/actor"
	"github.com/adrianozp/platform/pkg/apperror"
	"github.com/adrianozp/platform/pkg/scope"
)

const (
	DefaultTokenDays = 90
	MaxTokenDays     = 365
)

var tokenName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

type TokenDraft struct {
	Name          string
	Scopes        []string
	Teams         []string
	Bypass        bool
	ExpiresInDays int
}

type TokenGrant struct {
	Name      string
	Scopes    []scope.Scope
	TeamIDs   []string
	Bypass    bool
	ExpiresAt time.Time
}

type IssuedToken struct {
	Token  AccessToken
	Secret string
}

type Whoami struct {
	User  User
	Token actor.Token
	Teams []string
}

func (d TokenDraft) Check() ([]scope.Scope, error) {
	if !tokenName.MatchString(d.Name) {
		return nil, apperror.Invalid("a token name is 1-64 letters, digits, dots, dashes or underscores")
	}
	if len(d.Teams) == 0 {
		return nil, apperror.Invalid("pick at least one team")
	}
	if d.ExpiresInDays < 0 || d.ExpiresInDays > MaxTokenDays {
		return nil, apperror.Invalid(fmt.Sprintf("a token expires within 1-%d days", MaxTokenDays))
	}
	if len(d.Scopes) == 0 {
		return nil, apperror.Invalid("pick at least one scope")
	}
	return scope.Parse(d.Scopes)
}

func (d TokenDraft) ExpiresAt(now time.Time) time.Time {
	days := d.ExpiresInDays
	if days == 0 {
		days = DefaultTokenDays
	}
	return now.Add(time.Duration(days) * 24 * time.Hour)
}
