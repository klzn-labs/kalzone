package service

import (
	"context"
	"slices"

	catalog "github.com/klzn-labs/kalzone/app/catalog/entities"
	databaseEntities "github.com/klzn-labs/kalzone/app/database/entities"
	queueEntities "github.com/klzn-labs/kalzone/app/queue/entities"
)

const databaseUse = "PLATFORM_DB_* on deploy"

type Queues interface {
	List(ctx context.Context, environment, teamSlug string) ([]queueEntities.Queue, error)
}

type Databases interface {
	AttachedTo(ctx context.Context, serviceID string) ([]databaseEntities.Database, error)
}

type ResourceUse struct {
	Kind        string
	Badge       string
	Name        string
	Environment string
	Href        string
	Use         string
}

func (h ServiceHandler) resources(ctx context.Context, service catalog.Service) ([]ResourceUse, error) {
	dbs, err := h.databases.AttachedTo(ctx, service.ID)
	if err != nil {
		return nil, err
	}
	all, err := h.queues.List(ctx, "", "")
	if err != nil {
		return nil, err
	}
	uses := []ResourceUse{}
	for _, db := range dbs {
		uses = append(uses, ResourceUse{Kind: "Database", Badge: "badge-ok", Name: db.Name, Environment: db.Environment, Href: "/resources/databases/" + db.Environment + "/" + db.Name, Use: databaseUse})
	}
	return append(uses, queueUses(all, service.ID)...), nil
}

func queueUses(all []queueEntities.Queue, serviceID string) []ResourceUse {
	uses := []ResourceUse{}
	for _, q := range all {
		if use := queueUse(q, serviceID); use != "" {
			uses = append(uses, ResourceUse{Kind: "Queue", Badge: "badge-accent", Name: q.Name, Environment: q.Environment, Href: "/resources/queues/" + q.Environment + "/" + q.Name, Use: use})
		}
	}
	return uses
}

func queueUse(q queueEntities.Queue, serviceID string) string {
	switch {
	case q.ConsumerID == serviceID:
		return "Consumer"
	case slices.Contains(q.Publishers, serviceID):
		return "Publisher"
	}
	return ""
}
