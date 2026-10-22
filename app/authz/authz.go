package authz

import (
	"context"

	"github.com/klzn-labs/kalzone/pkg/actor"
	"github.com/klzn-labs/kalzone/pkg/apperror"
	"github.com/klzn-labs/kalzone/pkg/events"
)

type Action string

const (
	CreateService   Action = "service.create"
	EditService     Action = "service.edit"
	DeleteService   Action = "service.delete"
	TransferService Action = "service.transfer"
	EditConfig      Action = "config.edit"
	EditSecrets     Action = "secrets.edit"
	EditQueue       Action = "queue.edit"
	EditDatabase    Action = "database.edit"
	AdoptDatabase   Action = "database.adopt"
	CreateRelease   Action = "release.create"
	Deploy          Action = "deploy"
	ManageMembers   Action = "team.members"
	ManageTeams     Action = "teams.manage"
	ManageAdmins    Action = "admins.manage"
	ManageTokens    Action = "tokens.manage"
	ApproveRequest  Action = "request.approve"
	ReadLogs        Action = "logs.read"
)

type Gate string

const (
	Open          Gate = "open"
	Bypassed      Gate = "bypassed"
	Approved      Gate = "approved"
	NeedsApproval Gate = "needs_approval"
)

type Role string

const (
	RoleNone       Role = ""
	RoleMember     Role = "member"
	RoleMaintainer Role = "maintainer"
)

type Resource struct {
	TeamID            string
	DestinationTeamID string
	Environment       string
}

type MembershipReader interface {
	RoleOf(ctx context.Context, teamID, userID string) (string, error)
}

type ProtectionReader interface {
	IsProtected(ctx context.Context, environment string) (bool, error)
}

var (
	errUnauthenticated = apperror.New(apperror.CodeUnauthenticated, "sign in first")
	errForbidden       = apperror.Forbidden("you are not allowed to do this")
	errReadOnlyDemo    = apperror.Forbidden("the demo is read-only").WithHint("sign in to change your own team's services")
)

type Publisher interface {
	Publish(ctx context.Context, event events.Event)
}

type Policy struct {
	memberships  MembershipReader
	environments ProtectionReader
}

func NewPolicy(memberships MembershipReader, environments ProtectionReader) Policy {
	return Policy{memberships: memberships, environments: environments}
}

func (p Policy) Can(ctx context.Context, a actor.Actor, action Action, r Resource) error {
	_, err := Direct(p.Gate(ctx, a, action, r))
	return err
}

func (p Policy) Gate(ctx context.Context, a actor.Actor, action Action, r Resource) (Gate, error) {
	if !a.SignedIn() {
		return "", errUnauthenticated
	}
	if a.Guest {
		return "", errReadOnlyDemo
	}
	if err := p.ownerCan(ctx, a, action, r); err != nil {
		return "", err
	}
	if !a.ViaToken() {
		return Open, nil
	}
	return p.tokenGate(ctx, a, action, r)
}

func Direct(g Gate, err error) (Gate, error) {
	if err == nil && g == NeedsApproval {
		return "", errApprovalRequired
	}
	return g, err
}

func (p Policy) ownerCan(ctx context.Context, a actor.Actor, action Action, r Resource) error {
	if a.Admin || action == ManageTokens {
		return nil
	}
	required, err := p.requiredRole(ctx, action, r)
	if err != nil || required == RoleNone {
		return firstOr(err, errForbidden)
	}
	return p.requireRole(ctx, a, r.TeamID, required)
}

func (p Policy) requiredRole(ctx context.Context, action Action, r Resource) (Role, error) {
	switch action {
	case CreateService, EditService, CreateRelease, ReadLogs:
		return RoleMember, nil
	case DeleteService, TransferService, ManageMembers, ApproveRequest:
		return RoleMaintainer, nil
	case EditConfig, EditSecrets, EditQueue, EditDatabase, Deploy:
		return p.roleForEnvironment(ctx, r.Environment)
	}
	return RoleNone, nil
}

func (p Policy) roleForEnvironment(ctx context.Context, environment string) (Role, error) {
	protected, err := p.environments.IsProtected(ctx, environment)
	if err != nil {
		return RoleNone, err
	}
	if protected {
		return RoleMaintainer, nil
	}
	return RoleMember, nil
}

func (p Policy) requireRole(ctx context.Context, a actor.Actor, teamID string, required Role) error {
	if teamID == "" {
		return errForbidden
	}
	role, err := p.memberships.RoleOf(ctx, teamID, a.UserID)
	if err != nil {
		return err
	}
	if Role(role) == RoleMaintainer || (required == RoleMember && Role(role) == RoleMember) {
		return nil
	}
	return errForbidden
}

func firstOr(err, fallback error) error {
	if err != nil {
		return err
	}
	return fallback
}
