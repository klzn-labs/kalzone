package crons

import (
	"net/http"

	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type listParams struct {
	Environment string `query:"environment" description:"only crons of this environment"`
	Team        string `query:"team" description:"only crons of this team"`
	apispec.ListParams
}

type cronParams struct {
	Environment string `path:"env"`
	Name        string `path:"name"`
}

type cronListParams struct {
	Environment string `path:"env"`
	Name        string `path:"name"`
	apispec.ListParams
}

func cronOperations(h CronsHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "listCrons", Method: http.MethodGet, Path: "/crons", Summary: "List the crons you can see, by environment and name, with their next and last run",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[CronDTO]{}, Handler: h.List},
		{ID: "createCron", Method: http.MethodPost, Path: "/crons", Summary: "Create a cron: on its UTC schedule the platform POSTs the payload to the service's path, signed like a queue delivery so the SDK's queue.Handler verifies it",
			Scope: scope.Services, Gated: true, Body: createDTO{}, Status: http.StatusCreated, Response: CronDTO{}, Handler: h.Create},
		{ID: "getCron", Method: http.MethodGet, Path: "/crons/:env/:name", Summary: "Show a cron with its next and last run",
			Scope: scope.Read, Params: cronParams{}, Status: http.StatusOK, Response: CronDTO{}, Handler: h.Get},
		{ID: "updateCron", Method: http.MethodPatch, Path: "/crons/:env/:name", Summary: "Change a cron's schedule, path, payload or timeout; fields left out stay as they are",
			Scope: scope.Services, Gated: true, Params: cronParams{}, Body: changeDTO{}, Status: http.StatusOK, Response: CronDTO{}, Handler: h.Update},
		{ID: "deleteCron", Method: http.MethodDelete, Path: "/crons/:env/:name", Summary: "Delete a cron and its run history; a run already started finishes",
			Scope: scope.Services, Gated: true, Params: cronParams{}, Status: http.StatusNoContent, Handler: h.Delete},
	}
}

func controlOperations(h CronsHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "pauseCron", Method: http.MethodPost, Path: "/crons/:env/:name/pause", Summary: "Stop a cron's scheduled calls; run now still works",
			Scope: scope.Services, Params: cronParams{}, Status: http.StatusOK, Response: CronDTO{}, Handler: h.Pause},
		{ID: "resumeCron", Method: http.MethodPost, Path: "/crons/:env/:name/resume", Summary: "Start a paused cron's scheduled calls again from its next scheduled time",
			Scope: scope.Services, Params: cronParams{}, Status: http.StatusOK, Response: CronDTO{}, Handler: h.Resume},
		{ID: "runCron", Method: http.MethodPost, Path: "/crons/:env/:name/runs", Summary: "Call a cron's service now, once, outside its schedule; skipped if a run is still going",
			Scope: scope.Services, Params: cronParams{}, Status: http.StatusAccepted, Response: runRequestedDTO{}, Handler: h.Run},
		{ID: "listCronRuns", Method: http.MethodGet, Path: "/crons/:env/:name/runs", Summary: "List a cron's last 50 runs, newest first, with their status: succeeded, failed, timed_out or skipped",
			Scope: scope.Read, Params: cronListParams{}, Status: http.StatusOK, Response: apispec.Page[RunDTO]{}, Handler: h.Runs},
	}
}
