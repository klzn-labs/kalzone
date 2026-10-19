package approval

const (
	ShipsForward  = "forward"
	ShipsBackward = "backward"
)

type Ships struct {
	State        string        `json:"state" enum:"ahead,first,diverged,unavailable,failed"`
	Direction    string        `json:"direction" enum:"forward,backward"`
	From         int           `json:"from_release,omitempty"`
	To           int           `json:"to_release"`
	Releases     []ShipRelease `json:"releases"`
	Unattributed []ShipCommit  `json:"unattributed,omitempty"`
	Leaving      []ShipCommit  `json:"leaving,omitempty"`
	CommitCount  int           `json:"commit_count"`
	PullCount    int           `json:"pull_count"`
	Truncated    bool          `json:"truncated,omitempty"`
	CompareURL   string        `json:"compare_url,omitempty"`
	Error        string        `json:"error,omitempty"`
}

type ShipRelease struct {
	Number  int          `json:"number"`
	State   string       `json:"state"`
	Commits []ShipCommit `json:"commits"`
}

type ShipCommit struct {
	SHA        string `json:"sha"`
	Title      string `json:"title"`
	Type       string `json:"type"`
	Author     string `json:"author,omitempty"`
	PullNumber int    `json:"pull_number,omitempty"`
	PullTitle  string `json:"pull_title,omitempty"`
	PullURL    string `json:"pull_url,omitempty"`
}
