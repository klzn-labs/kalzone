package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/internal/queued/model"
	"github.com/klzn-labs/kalzone/internal/queued/registry"
)

const (
	maxDelaySeconds   = 86400
	maxIdempotencyKey = 128
)

type Queues interface {
	Lookup(name, tokenHash string) (model.Queue, registry.LookupResult)
}

type Publisher interface {
	Publish(ctx context.Context, q model.Queue, m model.Outgoing) (string, error)
}

type Server struct {
	queues    Queues
	publisher Publisher
}

func New(queues Queues, publisher Publisher) Server {
	return Server{queues: queues, publisher: publisher}
}

func (s Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/queues/{name}/messages", s.publish)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func (s Server) publish(w http.ResponseWriter, r *http.Request) {
	q, status := s.authorize(r)
	if status != 0 {
		fail(w, status, http.StatusText(status))
		return
	}
	m, status, problem := outgoing(r, q.Settings.MaxMessageBytes)
	if status != 0 {
		fail(w, status, problem)
		return
	}
	id, err := s.publisher.Publish(r.Context(), q, m)
	if err != nil {
		fail(w, http.StatusServiceUnavailable, "the queue backend is unavailable, retry")
		return
	}
	reply(w, http.StatusAccepted, map[string]string{"message_id": id})
}

func (s Server) authorize(r *http.Request) (model.Queue, int) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return model.Queue{}, http.StatusUnauthorized
	}
	sum := sha256.Sum256([]byte(token))
	q, result := s.queues.Lookup(r.PathValue("name"), hex.EncodeToString(sum[:]))
	switch result {
	case registry.Found:
		return q, 0
	case registry.Forbidden:
		return model.Queue{}, http.StatusForbidden
	case registry.Unauthorized:
		return model.Queue{}, http.StatusUnauthorized
	}
	return model.Queue{}, http.StatusNotFound
}

func outgoing(r *http.Request, maxBytes int) (model.Outgoing, int, string) {
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(maxBytes)+1))
	if err != nil {
		return model.Outgoing{}, http.StatusBadRequest, "the body could not be read"
	}
	if len(body) > maxBytes {
		return model.Outgoing{}, http.StatusRequestEntityTooLarge, "the message is larger than the queue allows"
	}
	key := r.Header.Get("Idempotency-Key")
	if len(key) > maxIdempotencyKey {
		return model.Outgoing{}, http.StatusBadRequest, "Idempotency-Key is longer than 128 characters"
	}
	due, ok := dueTime(r.Header.Get("Platform-Delay"))
	if !ok {
		return model.Outgoing{}, http.StatusBadRequest, "Platform-Delay must be whole seconds up to 86400"
	}
	return model.Outgoing{Body: body, ContentType: r.Header.Get("Content-Type"), IdempotencyKey: key, Due: due}, 0, ""
}

func dueTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, true
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 || seconds > maxDelaySeconds {
		return time.Time{}, false
	}
	return time.Now().Add(time.Duration(seconds) * time.Second), true
}

func fail(w http.ResponseWriter, status int, message string) {
	reply(w, status, map[string]string{"error": message})
}

func reply(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
