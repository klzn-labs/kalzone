package entities

type TeardownStatus string

const (
	TeardownRemoved          TeardownStatus = "removed"
	TeardownRemoving         TeardownStatus = "removing"
	TeardownAwaitingApproval TeardownStatus = "awaiting_approval"
)

type TeardownStep struct {
	Environment  string
	Status       TeardownStatus
	DeploymentID string
	RequestID    string
	ApproveURL   string
}

type Decommission struct {
	Service Service
	Steps   []TeardownStep
	Deleted bool
}
