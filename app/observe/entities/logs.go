package entities

import "time"

type LogLine struct {
	At        time.Time
	Container string
	Line      string
}

type LogSource struct {
	ID          string
	Slug        string
	OwnerTeamID string
}
