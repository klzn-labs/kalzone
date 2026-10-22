package orgs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/klzn-labs/kalzone/app/observe/entities"
)

func (u OrgsUsecase) apply(ctx context.Context, ids map[string]int, plan entities.OrgPlan) error {
	org, err := u.ensureOrg(ctx, ids, plan.Name)
	if err != nil {
		return fmt.Errorf("org %s: %w", plan.Name, err)
	}
	err = errors.Join(u.syncMembers(ctx, org, plan.Members), u.syncDatasources(ctx, org, plan.Datasources), u.syncDashboards(ctx, org, plan.Dashboards))
	if err != nil {
		return fmt.Errorf("org %s: %w", plan.Name, err)
	}
	return nil
}

func (u OrgsUsecase) ensureOrg(ctx context.Context, ids map[string]int, name string) (int, error) {
	if id, ok := ids[name]; ok {
		return id, nil
	}
	id, err := u.grafana.CreateOrg(ctx, name)
	if err == nil {
		ids[name] = id
	}
	return id, err
}

func (u OrgsUsecase) syncMembers(ctx context.Context, org int, want map[string]string) error {
	have, err := u.grafana.Members(ctx, org)
	if err != nil {
		return err
	}
	errs := []error{}
	present := map[string]bool{u.grafana.AdminLogin(): true}
	for _, m := range have {
		present[m.Login] = true
		errs = append(errs, u.reconcileMember(ctx, org, m, want))
	}
	for login, role := range want {
		if !present[login] {
			errs = append(errs, u.grafana.AddMember(ctx, org, login, role))
		}
	}
	return errors.Join(errs...)
}

func (u OrgsUsecase) reconcileMember(ctx context.Context, org int, m entities.OrgMember, want map[string]string) error {
	if m.Login == u.grafana.AdminLogin() {
		return nil
	}
	role, wanted := want[m.Login]
	if !wanted {
		return u.grafana.RemoveMember(ctx, org, m.UserID)
	}
	if role != m.Role {
		return u.grafana.SetRole(ctx, org, m.UserID, role)
	}
	return nil
}

func (u OrgsUsecase) syncDatasources(ctx context.Context, org int, datasources []entities.Datasource) error {
	errs := []error{}
	for _, d := range datasources {
		errs = append(errs, u.grafana.UpsertDatasource(ctx, org, d))
	}
	return errors.Join(errs...)
}

func (u OrgsUsecase) syncDashboards(ctx context.Context, org int, dashboards []map[string]any) error {
	if err := u.grafana.EnsureFolder(ctx, org); err != nil {
		return err
	}
	wanted := map[string]bool{}
	errs := []error{}
	for _, d := range dashboards {
		wanted[d["uid"].(string)] = true
		errs = append(errs, u.grafana.UpsertDashboard(ctx, org, d))
	}
	return errors.Join(append(errs, u.dropStale(ctx, org, wanted))...)
}

func (u OrgsUsecase) dropStale(ctx context.Context, org int, wanted map[string]bool) error {
	uids, err := u.grafana.DashboardUIDs(ctx, org)
	if err != nil {
		return err
	}
	errs := []error{}
	for _, uid := range uids {
		if !wanted[uid] {
			errs = append(errs, u.grafana.DeleteDashboard(ctx, org, uid))
		}
	}
	return errors.Join(errs...)
}

func (u OrgsUsecase) prune(ctx context.Context, existing []entities.Org, plans []entities.OrgPlan, ids map[string]int) error {
	planned := map[string]bool{}
	for _, p := range plans {
		planned[p.Name] = true
	}
	errs := []error{}
	for _, o := range existing {
		if managed(o.Name) && !planned[o.Name] {
			errs = append(errs, u.grafana.DeleteOrg(ctx, o.ID))
			delete(ids, o.Name)
		}
	}
	return errors.Join(errs...)
}

func managed(name string) bool {
	return strings.HasPrefix(name, entities.TeamOrgPrefix) || name == entities.DemoOrg
}
