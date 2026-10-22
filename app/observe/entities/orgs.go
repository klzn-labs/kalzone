package entities

const (
	MainOrg       = 1
	TeamOrgPrefix = "team-"
	DemoOrg       = "demo"
	RoleViewer    = "Viewer"
	RoleAdmin     = "Admin"
	RoleNone      = "None"
	TestDataUID   = "testdata"
)

type OrgTeam struct {
	ID      string
	Slug    string
	Demo    bool
	Members []string
}

type Person struct {
	Login  string
	Name   string
	Admin  bool
	OnTeam bool
}

type Datasource struct {
	UID      string
	Name     string
	Type     string
	URL      string
	Default  bool
	JSONData map[string]any
	Secure   map[string]string
}

type OrgPlan struct {
	Name        string
	Members     map[string]string
	Datasources []Datasource
	Dashboards  []map[string]any
}

type TelemetryLink struct {
	URL    string
	Tokens map[string]string
}

type OrgInputs struct {
	Teams     []OrgTeam
	People    []Person
	Services  []Service
	Simulated []Service
	Telemetry TelemetryLink
}

func TeamOrgName(teamSlug string) string {
	return TeamOrgPrefix + teamSlug
}

func OrgPlans(in OrgInputs) []OrgPlan {
	plans := []OrgPlan{}
	demo := false
	for _, t := range in.Teams {
		if t.Demo {
			demo = true
			continue
		}
		plans = append(plans, teamPlan(t, in))
	}
	if demo {
		plans = append(plans, demoPlan(in))
	}
	return plans
}

func teamPlan(t OrgTeam, in OrgInputs) OrgPlan {
	members := admins(in.People)
	for _, login := range t.Members {
		if _, admin := members[login]; !admin {
			members[login] = RoleViewer
		}
	}
	dashboards := []map[string]any{}
	for _, s := range in.Services {
		if s.TeamID == t.ID {
			dashboards = append(dashboards, Dashboard(s))
		}
	}
	datasources := TeamDatasources(in.Telemetry.URL, t.Slug, in.Telemetry.Tokens[t.Slug])
	return OrgPlan{Name: TeamOrgName(t.Slug), Members: members, Datasources: datasources, Dashboards: dashboards}
}

func demoPlan(in OrgInputs) OrgPlan {
	members := admins(in.People)
	for _, p := range in.People {
		if !p.Admin && !p.OnTeam {
			members[p.Login] = RoleViewer
		}
	}
	dashboards := make([]map[string]any, 0, len(in.Simulated))
	for _, s := range in.Simulated {
		dashboards = append(dashboards, DemoDashboard(s))
	}
	return OrgPlan{Name: DemoOrg, Members: members, Datasources: []Datasource{testData()}, Dashboards: dashboards}
}

func admins(people []Person) map[string]string {
	members := map[string]string{}
	for _, p := range people {
		if p.Admin {
			members[p.Login] = RoleAdmin
		}
	}
	return members
}

func TeamDatasources(baseURL, teamSlug, token string) []Datasource {
	base := baseURL + "/telemetry/" + teamSlug
	return []Datasource{
		withToken(Datasource{UID: "prometheus", Name: "Prometheus", Type: "prometheus", URL: base + "/prometheus", Default: true}, token),
		withToken(Datasource{UID: "loki", Name: "Loki", Type: "loki", URL: base + "/loki"}, token),
	}
}

func withToken(d Datasource, token string) Datasource {
	d.JSONData = map[string]any{"httpHeaderName1": "Authorization", "httpMethod": "POST"}
	d.Secure = map[string]string{"httpHeaderValue1": "Bearer " + token}
	return d
}

func testData() Datasource {
	return Datasource{UID: TestDataUID, Name: "TestData", Type: "grafana-testdata-datasource", Default: true, JSONData: map[string]any{}, Secure: map[string]string{}}
}

type Org struct {
	ID   int
	Name string
}

type OrgMember struct {
	UserID int
	Login  string
	Role   string
}

func MainOrgRoles(people []Person) map[string]string {
	roles := map[string]string{}
	for _, p := range people {
		roles[p.Login] = RoleNone
		if p.Admin {
			roles[p.Login] = RoleAdmin
		}
	}
	return roles
}
