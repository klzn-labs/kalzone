package entities

import "time"

type Role string

const (
	RoleMember     Role = "member"
	RoleMaintainer Role = "maintainer"
)

func (r Role) Valid() bool {
	return r == RoleMember || r == RoleMaintainer
}

func (r Role) AtLeast(required Role) bool {
	return r == RoleMaintainer || r == required
}

type Team struct {
	ID        string
	Slug      string
	Name      string
	CreatedAt time.Time
}

type Membership struct {
	TeamID    string
	UserID    string
	Role      Role
	CreatedAt time.Time
}

type UserRef struct {
	ID        string
	Login     string
	Name      string
	AvatarURL string
}

type Member struct {
	User      UserRef
	Role      Role
	CreatedAt time.Time
}
