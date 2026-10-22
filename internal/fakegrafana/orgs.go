package fakegrafana

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func (g *Grafana) routeOrgs(mux *http.ServeMux) {
	mux.HandleFunc("GET /grafana/api/orgs", g.admin(g.listOrgs))
	mux.HandleFunc("POST /grafana/api/orgs", g.admin(g.createOrg))
	mux.HandleFunc("DELETE /grafana/api/orgs/{id}", g.admin(g.deleteOrg))
	mux.HandleFunc("GET /grafana/api/users/lookup", g.admin(g.lookupUser))
	mux.HandleFunc("POST /grafana/api/admin/users", g.admin(g.createUser))
	mux.HandleFunc("GET /grafana/api/orgs/{id}/users", g.admin(g.listMembers))
	mux.HandleFunc("POST /grafana/api/orgs/{id}/users", g.admin(g.addMember))
	mux.HandleFunc("PATCH /grafana/api/orgs/{id}/users/{user}", g.admin(g.setRole))
	mux.HandleFunc("DELETE /grafana/api/orgs/{id}/users/{user}", g.admin(g.removeMember))
	mux.HandleFunc("GET /grafana/api/datasources/uid/{uid}", g.admin(g.getDatasource))
	mux.HandleFunc("POST /grafana/api/datasources", g.admin(g.putDatasource))
	mux.HandleFunc("PUT /grafana/api/datasources/uid/{uid}", g.admin(g.putDatasource))
}

func (g *Grafana) OrgID(name string) (int, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	id, ok := g.orgs[name]
	return id, ok
}

func (g *Grafana) Members(org int) map[string]string {
	g.mu.Lock()
	defer g.mu.Unlock()
	copied := map[string]string{}
	for login, role := range g.members[org] {
		copied[login] = role
	}
	return copied
}

func (g *Grafana) Datasource(org int, uid string) map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.datasources[org][uid]
}

func (g *Grafana) listOrgs(w http.ResponseWriter, _ *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	orgs := []map[string]any{}
	for name, id := range g.orgs {
		orgs = append(orgs, map[string]any{"id": id, "name": name})
	}
	_ = json.NewEncoder(w).Encode(orgs)
}

func (g *Grafana) createOrg(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	g.mu.Lock()
	defer g.mu.Unlock()
	id := len(g.dashboards) + 1
	g.orgs[body.Name], g.members[id], g.dashboards[id] = id, map[string]string{AdminUser: "Admin"}, map[string]string{}
	_ = json.NewEncoder(w).Encode(map[string]int{"orgId": id})
}

func (g *Grafana) deleteOrg(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	g.mu.Lock()
	defer g.mu.Unlock()
	for name, orgID := range g.orgs {
		if orgID == id {
			delete(g.orgs, name)
		}
	}
	delete(g.members, id)
}

func (g *Grafana) lookupUser(w http.ResponseWriter, r *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.users[r.URL.Query().Get("loginOrEmail")]; !ok {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (g *Grafana) createUser(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login string `json:"login"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.users[body.Login] = len(g.users) + 1
	g.members[1][body.Login] = "None"
}

func (g *Grafana) listMembers(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	g.mu.Lock()
	defer g.mu.Unlock()
	members := []map[string]any{}
	for login, role := range g.members[id] {
		members = append(members, map[string]any{"userId": g.users[login], "login": login, "role": role})
	}
	_ = json.NewEncoder(w).Encode(members)
}

func (g *Grafana) addMember(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Login string `json:"loginOrEmail"`
		Role  string `json:"role"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	id, _ := strconv.Atoi(r.PathValue("id"))
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.users[body.Login]; !ok {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	g.members[id][body.Login] = body.Role
}

func (g *Grafana) setRole(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Role string `json:"role"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	g.changeMember(r, func(members map[string]string, login string) { members[login] = body.Role })
}

func (g *Grafana) removeMember(w http.ResponseWriter, r *http.Request) {
	g.changeMember(r, func(members map[string]string, login string) { delete(members, login) })
}

func (g *Grafana) changeMember(r *http.Request, change func(map[string]string, string)) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	user, _ := strconv.Atoi(r.PathValue("user"))
	g.mu.Lock()
	defer g.mu.Unlock()
	for login, userID := range g.users {
		if userID == user {
			change(g.members[id], login)
		}
	}
}

func (g *Grafana) getDatasource(w http.ResponseWriter, r *http.Request) {
	if g.Datasource(orgOf(r), r.PathValue("uid")) == nil {
		w.WriteHeader(http.StatusNotFound)
	}
}

func (g *Grafana) putDatasource(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	uid, _ := body["uid"].(string)
	org := orgOf(r)
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.datasources[org] == nil {
		g.datasources[org] = map[string]map[string]any{}
	}
	g.datasources[org][uid] = body
}

func (g *Grafana) SetMember(org int, login, role string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.users[login]; !ok {
		g.users[login] = len(g.users) + 1
	}
	g.members[org][login] = role
}
