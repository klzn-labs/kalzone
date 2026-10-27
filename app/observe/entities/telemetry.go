package entities

const (
	AllTeams       = "all"
	PlatformTenant = "platform"
	LegacyTenant   = "fake"
	NoTenant       = "kalzone-none"
)

func TeamScope(services []Service, resources Resources, teamID string, environments []string) Scope {
	s := Scope{Services: []string{}, Environments: environments, MySQLUsers: []string{}, Schemas: []string{}, Queues: []string{}}
	for _, svc := range services {
		if svc.TeamID != teamID {
			continue
		}
		s.Services = append(s.Services, svc.Slug)
		s.MySQLUsers, s.Schemas = withDatabases(s.MySQLUsers, s.Schemas, svc.Databases)
	}
	return withResources(s, resources.OwnedBy(teamID))
}

func withResources(s Scope, owned Resources) Scope {
	s.MySQLUsers, s.Schemas = withDatabases(s.MySQLUsers, s.Schemas, owned.databases())
	for _, q := range owned.Queues {
		s.Queues = appendNew(s.Queues, q.ID)
	}
	return s
}

func withDatabases(users, schemas []string, databases []Database) ([]string, []string) {
	for _, db := range databases {
		users = appendNew(users, db.User)
		schemas = appendNew(schemas, db.Schema)
	}
	return users, schemas
}

func appendNew(values []string, value string) []string {
	for _, v := range values {
		if v == value {
			return values
		}
	}
	return append(values, value)
}

func AllTenants(services []Service, environments []string) []string {
	everyone := Scope{Environments: environments}
	for _, svc := range services {
		everyone.Services = append(everyone.Services, svc.Slug)
	}
	return append(append(everyone.Tenants(), PlatformComponents...), PlatformTenant, LegacyTenant)
}

type TelemetryAnswer struct {
	Status      int
	ContentType string
	Body        []byte
}

type LokiCall struct {
	Method      string
	Path        string
	RawQuery    string
	ContentType string
	Body        []byte
	Tenants     []string
}
