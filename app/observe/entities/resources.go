package entities

type OwnedDatabase struct {
	TeamID string
	Database
}

type OwnedQueue struct {
	TeamID string
	ID     string
}

type Resources struct {
	Databases []OwnedDatabase
	Queues    []OwnedQueue
}

func (r Resources) OwnedBy(teamID string) Resources {
	owned := Resources{}
	for _, db := range r.Databases {
		if db.TeamID == teamID {
			owned.Databases = append(owned.Databases, db)
		}
	}
	for _, q := range r.Queues {
		if q.TeamID == teamID {
			owned.Queues = append(owned.Queues, q)
		}
	}
	return owned
}

func (r Resources) databases() []Database {
	result := make([]Database, 0, len(r.Databases))
	for _, db := range r.Databases {
		result = append(result, db.Database)
	}
	return result
}
