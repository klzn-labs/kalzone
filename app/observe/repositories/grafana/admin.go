package grafana

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"

	"github.com/klzn-labs/kalzone/app/observe/entities"
)

func (c Client) Orgs(ctx context.Context) ([]entities.Org, error) {
	var orgs []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	status, err := c.call(ctx, entities.MainOrg, http.MethodGet, "/api/orgs?perpage=5000", nil, &orgs)
	if err := expect(status, err, http.StatusOK); err != nil {
		return nil, err
	}
	result := make([]entities.Org, 0, len(orgs))
	for _, o := range orgs {
		result = append(result, entities.Org{ID: o.ID, Name: o.Name})
	}
	return result, nil
}

func (c Client) CreateOrg(ctx context.Context, name string) (int, error) {
	var created struct {
		OrgID int `json:"orgId"`
	}
	status, err := c.call(ctx, entities.MainOrg, http.MethodPost, "/api/orgs", map[string]string{"name": name}, &created)
	return created.OrgID, expect(status, err, http.StatusOK)
}

func (c Client) DeleteOrg(ctx context.Context, id int) error {
	status, err := c.call(ctx, entities.MainOrg, http.MethodDelete, "/api/orgs/"+strconv.Itoa(id), nil, nil)
	if status == http.StatusNotFound {
		return nil
	}
	return expect(status, err, http.StatusOK)
}

func (c Client) EnsureUser(ctx context.Context, p entities.Person) error {
	path := "/api/users/lookup?loginOrEmail=" + url.QueryEscape(p.Login)
	status, err := c.call(ctx, entities.MainOrg, http.MethodGet, path, nil, nil)
	if err != nil || status == http.StatusOK {
		return err
	}
	body := map[string]string{"login": p.Login, "name": nameOf(p), "password": randomPassword()}
	status, err = c.call(ctx, entities.MainOrg, http.MethodPost, "/api/admin/users", body, nil)
	return expect(status, err, http.StatusOK)
}

func nameOf(p entities.Person) string {
	if p.Name != "" {
		return p.Name
	}
	return p.Login
}

func randomPassword() string {
	raw := make([]byte, 24)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func (c Client) Members(ctx context.Context, org int) ([]entities.OrgMember, error) {
	var users []struct {
		UserID int    `json:"userId"`
		Login  string `json:"login"`
		Role   string `json:"role"`
	}
	status, err := c.call(ctx, org, http.MethodGet, "/api/orgs/"+strconv.Itoa(org)+"/users", nil, &users)
	if err := expect(status, err, http.StatusOK); err != nil {
		return nil, err
	}
	members := make([]entities.OrgMember, 0, len(users))
	for _, u := range users {
		members = append(members, entities.OrgMember{UserID: u.UserID, Login: u.Login, Role: u.Role})
	}
	return members, nil
}

func (c Client) AddMember(ctx context.Context, org int, login, role string) error {
	body := map[string]string{"loginOrEmail": login, "role": role}
	status, err := c.call(ctx, org, http.MethodPost, "/api/orgs/"+strconv.Itoa(org)+"/users", body, nil)
	return expect(status, err, http.StatusOK)
}

func (c Client) SetRole(ctx context.Context, org, userID int, role string) error {
	path := "/api/orgs/" + strconv.Itoa(org) + "/users/" + strconv.Itoa(userID)
	status, err := c.call(ctx, org, http.MethodPatch, path, map[string]string{"role": role}, nil)
	return expect(status, err, http.StatusOK)
}

func (c Client) RemoveMember(ctx context.Context, org, userID int) error {
	path := "/api/orgs/" + strconv.Itoa(org) + "/users/" + strconv.Itoa(userID)
	status, err := c.call(ctx, org, http.MethodDelete, path, nil, nil)
	return expect(status, err, http.StatusOK)
}

func (c Client) UpsertDatasource(ctx context.Context, org int, d entities.Datasource) error {
	body := map[string]any{
		"uid": d.UID, "name": d.Name, "type": d.Type, "url": d.URL, "access": "proxy",
		"isDefault": d.Default, "jsonData": d.JSONData, "secureJsonData": d.Secure,
	}
	status, err := c.call(ctx, org, http.MethodGet, "/api/datasources/uid/"+url.PathEscape(d.UID), nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		status, err = c.call(ctx, org, http.MethodPost, "/api/datasources", body, nil)
		return expect(status, err, http.StatusOK)
	}
	status, err = c.call(ctx, org, http.MethodPut, "/api/datasources/uid/"+url.PathEscape(d.UID), body, nil)
	return expect(status, err, http.StatusOK)
}
