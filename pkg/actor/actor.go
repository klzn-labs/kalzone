package actor

import (
	"slices"
	"time"

	"github.com/adrianozp/platform/pkg/scope"
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
	return !a.ViaToken() || slices.Contains(a.Token.TeamIDs, teamID)
}

func (a Actor) Approved() bool {
	return a.Approval.RequestID != ""
}

func (a Actor) SessionAdmin() bool {
	return a.Admin && !a.ViaToken()
}
