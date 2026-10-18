package environment

import (
	"context"

	envconfig "github.com/klzn-labs/kalzone/app/envconfig/entities"
	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const historyLimit = 20

var errUnknownEnvironment = apperror.NotFound("environment not found")

func (h EnvironmentHandler) history(ctx context.Context, serviceSlug, envSlug string) ([]RevisionView, error) {
	revisions, err := h.configs.History(ctx, serviceSlug, envSlug, historyLimit)
	if err != nil {
		return nil, err
	}
	authors, err := h.views.UsersByID(ctx, authorIDs(revisions))
	if err != nil {
		return nil, err
	}
	views := make([]RevisionView, 0, len(revisions))
	for i, rev := range revisions {
		views = append(views, RevisionView{Revision: rev, Author: authors[rev.AuthorID], Changes: envconfig.Diff(previous(revisions, i), rev)})
	}
	return views, nil
}

func previous(revisions []envconfig.Revision, i int) envconfig.Revision {
	if i+1 < len(revisions) {
		return revisions[i+1]
	}
	return envconfig.Initial(revisions[i].ServiceID, revisions[i].Environment)
}

func authorIDs(revisions []envconfig.Revision) []string {
	ids := make([]string, 0, len(revisions))
	for _, r := range revisions {
		ids = append(ids, r.AuthorID)
	}
	return ids
}

func (h EnvironmentHandler) secretRows(ctx context.Context, serviceSlug, envSlug string) ([]SecretRow, error) {
	infos, err := h.configs.Secrets(ctx, serviceSlug, envSlug)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(infos))
	for _, info := range infos {
		ids = append(ids, info.SetBy)
	}
	users, err := h.views.UsersByID(ctx, ids)
	if err != nil {
		return nil, err
	}
	rows := make([]SecretRow, 0, len(infos))
	for _, info := range infos {
		rows = append(rows, SecretRow{Name: info.Name, SetBy: users[info.SetBy], Info: info})
	}
	return rows, nil
}
