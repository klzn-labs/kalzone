package fakeregistry

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

func (r *Registry) Pushable(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pushable[name] = true
}

func (r *Registry) token(w http.ResponseWriter, req *http.Request) {
	if _, _, ok := req.BasicAuth(); !ok {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	name := strings.TrimSuffix(strings.TrimPrefix(req.URL.Query().Get("scope"), "repository:"), ":push,pull")
	actions := []string{"pull"}
	r.mu.Lock()
	if r.pushable[name] {
		actions = append(actions, "push")
	}
	r.mu.Unlock()
	claims, _ := json.Marshal(map[string]any{"access": []map[string]any{{"type": "repository", "name": name, "actions": actions}}})
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": "h." + base64.RawURLEncoding.EncodeToString(claims) + ".s"})
}
