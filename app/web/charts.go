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
	Columns   int
	Panels    []ChartPanel
	Dashboard string
}

type ChartPanel struct {
	Title string
	Wide  bool
	Light string
	Dark  string
}

func TeamCharts(orgs GrafanaOrgs, teamSlug, uid string, embeds []observe.EmbeddedPanel, vars url.Values) Charts {
	org, known := orgs.OrgID(observe.TeamOrgName(teamSlug))
	if !orgs.Enabled() || !known {
		return Charts{}
	}
	vars.Set("orgId", strconv.Itoa(org))
	panels := make([]ChartPanel, 0, len(embeds))
	for _, e := range embeds {
		panel := chartPanel(uid, e.ID, vars)
		panel.Wide = e.Wide
		panels = append(panels, panel)
	}
	return Charts{Enabled: true, Columns: columnsFor(embeds), Panels: panels, Dashboard: "/grafana/d/" + uid + "?" + vars.Encode()}
}

func columnsFor(embeds []observe.EmbeddedPanel) int {
	narrow := 0
	for _, e := range embeds {
		if !e.Wide {
			narrow++
		}
	}
	if narrow == 3 {
		return 3
	}
	return 2
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
	return TeamCharts(orgs, teamSlug, observe.DatabaseDashboardUID, observe.DatabaseEmbeds, url.Values{"var-schema": {schema}, "var-user": {user}})
}

func QueueCharts(orgs GrafanaOrgs, teamSlug, queueID string) Charts {
	return TeamCharts(orgs, teamSlug, observe.QueueDashboardUID, observe.QueueEmbeds, url.Values{"var-queue_id": {queueID}})
}
