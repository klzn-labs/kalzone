package entities

import "time"

type Kind string

const (
	KindQueue    Kind = "queue"
	KindDatabase Kind = "database"
	KindCron     Kind = "cron"
)

type Resource struct {
	Kind        Kind
	Name        string
	Environment string
	TeamID      string
	Attached    []string
	CreatedAt   time.Time
}
