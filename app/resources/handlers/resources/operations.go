package resources

import (
	"net/http"

	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type listParams struct {
	Kind        string `query:"kind" description:"only resources of this kind: queue or database"`
	Environment string `query:"environment" description:"only resources of this environment"`
	apispec.ListParams
}

func resourceOperations(h ResourcesHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "listResources", Method: http.MethodGet, Path: "/resources", Summary: "List the managed resources you can see, such as queues, by environment, kind and name",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[ResourceDTO]{}, Handler: h.List},
	}
}
