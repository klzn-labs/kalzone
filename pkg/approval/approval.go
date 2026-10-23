package approval

import "time"

const (
	KindConfigRevise   = "config.revise"
	KindSecretSet      = "secret.set"
	KindSecretUnset    = "secret.unset"
	KindDeploy         = "deploy"
	KindPromote        = "promote"
	KindRollback       = "rollback"
	KindConfigApply    = "config.apply"
	KindRemove         = "remove"
	KindQueueCreate    = "queue.create"
	KindQueueUpdate    = "queue.update"
	KindQueueDelete    = "queue.delete"
	KindDatabaseCreate = "database.create"
	KindDatabaseAttach = "database.attach"
	KindDatabaseDetach = "database.detach"
	KindDatabaseDelete = "database.delete"
	KindCronCreate     = "cron.create"
	KindCronUpdate     = "cron.update"
	KindCronDelete     = "cron.delete"
)

type Change struct {
	Kind        string
	Subject     string
	ServiceID   string
	ServiceSlug string
	Resource    string
	TeamID      string
	Environment string
	Summary     string
	Proposal    any
}

type Pending struct {
	Kind       string    `json:"kind"`
	ID         string    `json:"id"`
	Status     string    `json:"status"`
	Summary    string    `json:"summary"`
	ApproveURL string    `json:"approve_url"`
	Poll       string    `json:"poll"`
	ExpiresAt  time.Time `json:"expires_at"`
}

func (p *Pending) Error() string {
	return "change request " + p.ID + " waits for a human to approve it at " + p.ApproveURL
}

type Line struct {
	Key    string
	Before string
	After  string
}

type Preview struct {
	Lines    []Line
	Moved    string
	Conflict string
	Ships    *Ships
}
