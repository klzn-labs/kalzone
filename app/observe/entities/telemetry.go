package entities

const (
	AllTeams       = "all"
	PlatformTenant = "platform"
	LegacyTenant   = "fake"
	NoTenant       = "kalzone-none"
)

func TeamScope(services []Service, teamID string, environments []string) Scope {
	s := Scope{Services: []string{}, Environments: environments, MySQLUsers: []string{}, Schemas: []string{}}
	for _, svc := range services {
		if svc.TeamID != teamID {
			continue
		}
		s.Services = append(s.Services, svc.Slug)
		s.MySQLUsers, s.Schemas = withDatabases(s.MySQLUsers, s.Schemas, svc.Databases)
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
