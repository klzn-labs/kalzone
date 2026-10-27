package web

import (
	"net/url"
	"strconv"

	observe "github.com/klzn-labs/kalzone/app/observe/entities"
)

type GrafanaOrgs interface {
	Enabled() bool
	OrgID(name string) (int, bool)
}

type Charts struct {
	Enabled   bool
	Panels    []ChartPanel
	Dashboard string
}

type ChartPanel struct {
	Title string
	Light string
	Dark  string
}

func TeamCharts(orgs GrafanaOrgs, teamSlug, uid string, vars url.Values) Charts {
	org, known := orgs.OrgID(observe.TeamOrgName(teamSlug))
	if !orgs.Enabled() || !known {
		return Charts{}
	}
	vars.Set("orgId", strconv.Itoa(org))
	panels := make([]ChartPanel, 0, len(observe.EmbeddedResourcePanels))
	for _, id := range observe.EmbeddedResourcePanels {
		panels = append(panels, chartPanel(uid, id, vars))
	}
	return Charts{Enabled: true, Panels: panels, Dashboard: "/grafana/d/" + uid + "?" + vars.Encode()}
}

func chartPanel(uid string, id int, vars url.Values) ChartPanel {
	src := func(theme string) string {
		q := url.Values{"panelId": {strconv.Itoa(id)}, "from": {"now-6h"}, "to": {"now"}, "theme": {theme}}
		for name, values := range vars {
			q[name] = values
		}
		return "/grafana/d-solo/" + uid + "?" + q.Encode()
	}
	return ChartPanel{Title: "panel " + strconv.Itoa(id), Light: src("light"), Dark: src("dark")}
}

func DatabaseCharts(orgs GrafanaOrgs, teamSlug, schema, user string) Charts {
	return TeamCharts(orgs, teamSlug, observe.DatabaseDashboardUID, url.Values{"var-schema": {schema}, "var-user": {user}})
}

func QueueCharts(orgs GrafanaOrgs, teamSlug, queueID string) Charts {
	return TeamCharts(orgs, teamSlug, observe.QueueDashboardUID, url.Values{"var-queue_id": {queueID}})
}
