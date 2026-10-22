package grafana_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/klzn-labs/kalzone/app/observe/entities"
	"github.com/klzn-labs/kalzone/app/observe/repositories/grafana"
	"github.com/klzn-labs/kalzone/pkg/config"
	"github.com/stretchr/testify/require"
)

type call struct {
	method, path, org string
	body              map[string]any
}

func recorder(t *testing.T, answers map[string]int) (grafana.Client, *[]call) {
	t.Helper()
	calls := &[]call{}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c := call{method: r.Method, path: r.URL.RequestURI(), org: r.Header.Get("X-Grafana-Org-Id")}
		_ = json.NewDecoder(r.Body).Decode(&c.body)
		*calls = append(*calls, c)
		if status, ok := answers[r.Method+" "+r.URL.Path]; ok {
			w.WriteHeader(status)
		}
		_, _ = w.Write([]byte(`[{"userId":7,"login":"bob","role":"Viewer"}]`))
	}))
	t.Cleanup(s.Close)
	return grafana.NewClient(config.Config{Grafana: config.Grafana{URL: s.URL, AdminUser: "admin", AdminPassword: "secret"}}), calls
}

func Test_EnsureUser_CreatesOnlyAMissingLogin(t *testing.T) {
	c, calls := recorder(t, map[string]int{"GET /api/users/lookup": http.StatusNotFound})

	require.NoError(t, c.EnsureUser(context.Background(), entities.Person{Login: "bob", Name: "Bob"}))

	require.Len(t, *calls, 2)
	require.Equal(t, "/api/admin/users", (*calls)[1].path)
	require.Equal(t, "bob", (*calls)[1].body["login"])
	require.Len(t, (*calls)[1].body["password"], 48)
}

func Test_UpsertDatasource_CreatesWhenMissingAndUpdatesInPlaceOtherwise(t *testing.T) {
	missing, created := recorder(t, map[string]int{"GET /api/datasources/uid/loki": http.StatusNotFound})
	present, updated := recorder(t, nil)
	d := entities.Datasource{UID: "loki", Type: "loki", URL: "http://x", Secure: map[string]string{"httpHeaderValue1": "Bearer t"}}

	require.NoError(t, missing.UpsertDatasource(context.Background(), 4, d))
	require.NoError(t, present.UpsertDatasource(context.Background(), 4, d))

	require.Equal(t, "POST", (*created)[1].method)
	require.Equal(t, "4", (*created)[1].org)
	require.Equal(t, "PUT /api/datasources/uid/loki", (*updated)[1].method+" "+(*updated)[1].path)
	require.Equal(t, map[string]any{"httpHeaderValue1": "Bearer t"}, (*updated)[1].body["secureJsonData"])
}

func Test_Members_ReadsTheOrgsUsersInsideThatOrg(t *testing.T) {
	c, calls := recorder(t, nil)

	members, err := c.Members(context.Background(), 3)

	require.NoError(t, err)
	require.Equal(t, []entities.OrgMember{{UserID: 7, Login: "bob", Role: "Viewer"}}, members)
	require.Equal(t, "/api/orgs/3/users", (*calls)[0].path)
	require.Equal(t, "3", (*calls)[0].org)
}
