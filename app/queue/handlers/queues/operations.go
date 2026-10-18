package queues

import (
	"net/http"

	"github.com/klzn-labs/kalzone/app/queue/entities"
	"github.com/klzn-labs/kalzone/internal/apispec"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

type listParams struct {
	Environment string `query:"environment" description:"only queues of this environment"`
	Team        string `query:"team" description:"only queues of this team"`
	apispec.ListParams
}

type queueParams struct {
	Environment string `path:"env"`
	Name        string `path:"name"`
}

type deleteParams struct {
	Environment string `path:"env"`
	Name        string `path:"name"`
	Force       bool   `query:"force" description:"delete even when messages wait"`
}

type queueListParams struct {
	Environment string `path:"env"`
	Name        string `path:"name"`
	apispec.ListParams
}

type messageParams struct {
	Environment string `path:"env"`
	Name        string `path:"name"`
	ID          string `path:"id"`
}

func queueOperations(h QueuesHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "listQueues", Method: http.MethodGet, Path: "/queues", Summary: "List the queues you can see, by environment and name",
			Scope: scope.Read, Params: listParams{}, Status: http.StatusOK, Response: apispec.Page[QueueDTO]{}, Handler: h.List},
		{ID: "createQueue", Method: http.MethodPost, Path: "/queues", Summary: "Create a queue delivering to a consumer service",
			Scope: scope.Services, Gated: true, Body: createDTO{}, Status: http.StatusCreated, Response: QueueDTO{}, Handler: h.Create},
		{ID: "getQueue", Method: http.MethodGet, Path: "/queues/:env/:name", Summary: "Show a queue",
			Scope: scope.Read, Params: queueParams{}, Status: http.StatusOK, Response: QueueDTO{}, Handler: h.Get},
		{ID: "updateQueue", Method: http.MethodPatch, Path: "/queues/:env/:name", Summary: "Change a queue's settings or publishers from a base version",
			Scope: scope.Services, Gated: true, Params: queueParams{}, Body: changeDTO{}, Status: http.StatusOK, Response: QueueDTO{}, Handler: h.Update},
		{ID: "deleteQueue", Method: http.MethodDelete, Path: "/queues/:env/:name", Summary: "Delete a queue",
			Scope: scope.Services, Gated: true, Params: deleteParams{}, Status: http.StatusNoContent, Handler: h.Delete},
	}
}

func controlOperations(h QueuesHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "pauseQueue", Method: http.MethodPost, Path: "/queues/:env/:name/pause", Summary: "Stop delivering a queue's messages",
			Scope: scope.Services, Params: queueParams{}, Status: http.StatusOK, Response: QueueDTO{}, Handler: h.Pause},
		{ID: "resumeQueue", Method: http.MethodPost, Path: "/queues/:env/:name/resume", Summary: "Deliver a paused queue's messages again",
			Scope: scope.Services, Params: queueParams{}, Status: http.StatusOK, Response: QueueDTO{}, Handler: h.Resume},
		{ID: "purgeDeadLetters", Method: http.MethodDelete, Path: "/queues/:env/:name/dead", Summary: "Drop every dead letter",
			Scope: scope.Services, Params: queueParams{}, Status: http.StatusAccepted, Handler: h.PurgeDead},
		{ID: "replayDeadLetters", Method: http.MethodPost, Path: "/queues/:env/:name/dead/replay-all", Summary: "Deliver every dead letter again",
			Scope: scope.Services, Params: queueParams{}, Status: http.StatusAccepted, Handler: h.ReplayAll},
		{ID: "replayDeadLetter", Method: http.MethodPost, Path: "/queues/:env/:name/dead/:id/replay", Summary: "Deliver one dead letter again",
			Scope: scope.Services, Params: messageParams{}, Status: http.StatusAccepted, Handler: h.Replay},
	}
}

func observeOperations(h QueuesHandler) []apispec.Operation {
	return []apispec.Operation{
		{ID: "getQueueStats", Method: http.MethodGet, Path: "/queues/:env/:name/stats", Summary: "Show a queue's depth and rates as the gateway last reported them; fresh false means no recent gateway report, so ignore the numbers",
			Scope: scope.Read, Params: queueParams{}, Status: http.StatusOK, Response: statsDTO{}, Handler: h.Stats},
		{ID: "listQueueVersions", Method: http.MethodGet, Path: "/queues/:env/:name/versions", Summary: "List a queue's settings versions, newest first",
			Scope: scope.Read, Params: queueListParams{}, Status: http.StatusOK, Response: apispec.Page[versionDTO]{}, Handler: h.Versions},
		{ID: "listDeadLetters", Method: http.MethodGet, Path: "/queues/:env/:name/dead", Summary: "List the newest dead letters",
			Scope: scope.Read, Params: queueListParams{}, Status: http.StatusOK, Response: apispec.Page[entities.DeadLetter]{}, Handler: h.Dead},
	}
}
