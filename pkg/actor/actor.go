package actor

import (
	"slices"
	"time"

	"github.com/klzn-labs/kalzone/pkg/scope"
)

type Token struct {
	ID        string
	Name      string
	Prefix    string
	Scopes    []scope.Scope
	TeamIDs   []string
	Bypass    bool
	ExpiresAt time.Time
}

func (t Token) Has(s scope.Scope) bool {
	return slices.Contains(t.Scopes, s)
}

type Approval struct {
	RequestID     string
	ApproverID    string
	ApproverLogin string
}

type Actor struct {
	UserID   string
	Login    string
	Admin    bool
	Guest    bool
	Teams    []string
	Token    Token
	Approval Approval
}

func (a Actor) SignedIn() bool {
	return a.UserID != ""
}

func (a Actor) ViaToken() bool {
	return a.Token.ID != ""
}

func (a Actor) Sees(teamID string) bool {
	return a.TokenGrants(teamID) && a.memberSees(teamID)
}

func (a Actor) TokenGrants(teamID string) bool {
	return !a.ViaToken() || slices.Contains(a.Token.TeamIDs, teamID)
}

func (a Actor) memberSees(teamID string) bool {
	return a.Unscoped() || slices.Contains(a.Teams, teamID)
}

func (a Actor) Unscoped() bool {
	return !a.SignedIn() || a.Admin
}

func (a Actor) TeamScope() ([]string, bool) {
	if a.ViaToken() {
		return slices.DeleteFunc(slices.Clone(a.Token.TeamIDs), func(id string) bool { return !a.memberSees(id) }), false
	}
	return a.Teams, a.Unscoped()
}

func (a Actor) Approved() bool {
	return a.Approval.RequestID != ""
}

func (a Actor) SessionAdmin() bool {
	return a.Admin && !a.ViaToken()
}
