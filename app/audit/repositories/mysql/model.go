package mysql

import (
	"encoding/json"
	"time"

	"github.com/adrianozp/platform/app/audit/entities"
)

type auditEvent struct {
	ID          string `gorm:"primaryKey"`
	At          time.Time
	ActorID     string
	ActorLogin  string
	Action      string
	TargetKind  string
	TargetID    string
	TargetName  string
	TeamID      string
	Environment string
	Details     []byte
}

func (auditEvent) TableName() string { return "audit_events" }

func fromEvent(e entities.Event) (auditEvent, error) {
	details, err := json.Marshal(nonNil(e.Details))
	return auditEvent{
		ID: e.ID, At: e.At, ActorID: e.ActorID, ActorLogin: e.ActorLogin, Action: e.Action,
		TargetKind: e.TargetKind, TargetID: e.TargetID, TargetName: e.TargetName,
		TeamID: e.TeamID, Environment: e.Environment, Details: details,
	}, err
}

func (m auditEvent) toEntity() (entities.Event, error) {
	e := entities.Event{
		ID: m.ID, At: m.At, ActorID: m.ActorID, ActorLogin: m.ActorLogin, Action: m.Action,
		TargetKind: m.TargetKind, TargetID: m.TargetID, TargetName: m.TargetName,
		TeamID: m.TeamID, Environment: m.Environment,
	}
	return e, json.Unmarshal(m.Details, &e.Details)
}

func nonNil(details map[string]any) map[string]any {
	if details == nil {
		return map[string]any{}
	}
	return details
}
