package web

import (
	"fmt"
	"time"

	audit "github.com/adrianozp/platform/app/audit/entities"
)

type Activity struct {
	ActorLogin  string
	Phrase      string
	TargetName  string
	Link        string
	Environment string
	Detail      string
	At          time.Time
}

type phrasing struct {
	phrase string
	link   func(e audit.Event) string
	detail func(e audit.Event) string
}

var phrasings = map[string]phrasing{
	"team.created":        {"created team", teamLink, nil},
	"team.deleted":        {"deleted team", nil, nil},
	"team.member_put":     {"changed membership in", teamLink, memberDetail},
	"team.member_removed": {"removed a member from", teamLink, removedDetail},
	"service.created":     {"created service", serviceLink, nil},
	"service.updated":     {"updated", serviceLink, nil},
	"service.transferred": {"moved", serviceLink, transferDetail},
	"service.deleted":     {"deleted service", nil, nil},
	"config.revised":      {"changed the config of", configLink, revisionDetail},
	"user.admin_granted":  {"made an admin:", nil, nil},
	"user.admin_revoked":  {"removed admin rights from", nil, nil},
}

func ActivityFrom(events []audit.Event) []Activity {
	result := make([]Activity, 0, len(events))
	for _, e := range events {
		result = append(result, activityOf(e))
	}
	return result
}

func activityOf(e audit.Event) Activity {
	a := Activity{ActorLogin: e.ActorLogin, Phrase: e.Action, TargetName: e.TargetName, Environment: e.Environment, At: e.At}
	p, known := phrasings[e.Action]
	if !known {
		return a
	}
	a.Phrase = p.phrase
	if p.link != nil {
		a.Link = p.link(e)
	}
	if p.detail != nil {
		a.Detail = p.detail(e)
	}
	return a
}

func teamLink(e audit.Event) string    { return "/teams/" + e.TargetName }
func serviceLink(e audit.Event) string { return "/services/" + e.TargetName }
func configLink(e audit.Event) string {
	return "/services/" + e.TargetName + "/env/" + e.Environment
}

func memberDetail(e audit.Event) string {
	return fmt.Sprintf("@%v → %v", e.Details["login"], e.Details["role"])
}

func removedDetail(e audit.Event) string {
	return fmt.Sprintf("@%v", e.Details["login"])
}

func transferDetail(e audit.Event) string {
	return fmt.Sprintf("to %v", e.Details["to_team"])
}

func revisionDetail(e audit.Event) string {
	return fmt.Sprintf("revision %v", e.Details["revision"])
}
