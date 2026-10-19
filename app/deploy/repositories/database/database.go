package database

import "context"

type Databases interface {
	Access(ctx context.Context, serviceID, environment string) (env, secrets map[string]string, err error)
}

type Access struct {
	databases Databases
}

func NewAccess(databases Databases) Access {
	return Access{databases: databases}
}

func (a Access) DatabaseAccess(ctx context.Context, serviceID, environment string) (map[string]string, map[string]string, error) {
	return a.databases.Access(ctx, serviceID, environment)
}
