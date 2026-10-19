package databases

import (
	"net/http"

	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type listParams struct {
	Environment string `query:"environment" description:"only databases of this environment"`
	Team        string `query:"team" description:"only databases of this team"`
	apispec.ListParams
}

type databaseParams struct {
	Environment string `path:"env"`
	Name        string `path:"name"`
}

type attachmentParams struct {
	Environment string `path:"env"`
	Name        string `path:"name"`
	Service     string `path:"service"`
}

func databaseOperations(h DatabasesHandler) []apispec.Operation {
	return append(readOperations(h), writeOperations(h)...)
}

func readOperations(h DatabasesHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "listDatabases", Method: http.MethodGet, Path: "/databases", Summary: "List the managed MySQL databases you can see, by environment and name",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[DatabaseDTO]{}, Handler: h.List},
		{ID: "getDatabase", Method: http.MethodGet, Path: "/databases/:env/:name", Summary: "Show a database's connection facts and attached services; the password is never shown",
			Scope: scope.Read, Params: databaseParams{}, Status: http.StatusOK, Response: DatabaseDTO{}, Handler: h.Get},
	}
}

func writeOperations(h DatabasesHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "createDatabase", Method: http.MethodPost, Path: "/databases", Summary: "Create a managed MySQL database: a schema and a user that may only use it",
			Scope: scope.Services, Gated: true, Body: createDTO{}, Status: http.StatusCreated, Response: DatabaseDTO{}, Handler: h.Create},
		{ID: "attachDatabase", Method: http.MethodPost, Path: "/databases/:env/:name/attachments", Summary: "Give a service the database's credentials as PLATFORM_DB_* values; takes effect on its next deploy; the token also needs the secrets scope on the service's team",
			Scope: scope.Services, Gated: true, Params: databaseParams{}, Body: attachDTO{}, Status: http.StatusOK, Response: DatabaseDTO{}, Handler: h.Attach},
		{ID: "detachDatabase", Method: http.MethodDelete, Path: "/databases/:env/:name/attachments/:service", Summary: "Stop giving a service the database's credentials; takes effect on its next deploy; the token also needs the secrets scope on the service's team",
			Scope: scope.Services, Gated: true, Params: attachmentParams{}, Status: http.StatusOK, Response: DatabaseDTO{}, Handler: h.Detach},
		{ID: "deleteDatabase", Method: http.MethodDelete, Path: "/databases/:env/:name", Summary: "Drop a database's schema, user and data; refused while a service is attached or still runs with it",
			Scope: scope.Services, Gated: true, Params: databaseParams{}, Status: http.StatusNoContent, Handler: h.Delete},
	}
}
