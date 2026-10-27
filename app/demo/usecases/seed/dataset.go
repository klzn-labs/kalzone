package seed

import team "github.com/klzn-labs/kalzone/app/team/entities"

type person struct {
	id    string
	login string
	name  string
	role  team.Role
}

var people = []person{
	{id: "00000000-0000-4000-8000-000000000011", login: "ana.demo", name: "Ana Ribeiro", role: team.RoleMaintainer},
	{id: "00000000-0000-4000-8000-000000000012", login: "mia.demo", name: "Mia Santos", role: team.RoleMaintainer},
	{id: "00000000-0000-4000-8000-000000000013", login: "bruno.demo", name: "Bruno Costa", role: team.RoleMember},
}

type workload struct {
	slug        string
	name        string
	description string
	releases    int
	prod        int
	rolledBack  int
	env         map[string]string
}

var workloads = []workload{
	{
		slug: "demo-shop-api", name: "Shop API", description: "Serves the catalog, carts and orders to the storefront.",
		releases: 18, prod: 17, rolledBack: 18, env: map[string]string{"LOG_LEVEL": "info", "CURRENCY": "EUR", "CART_TTL": "72h"},
	},
}

var titles = []string{
	"feat: gift cards at checkout",
	"fix: keep the cart when a session expires",
	"feat: show delivery dates on the product page",
	"fix: round VAT per line, not per order",
	"perf: cache category pages for a minute",
	"feat: let customers reorder a past order",
	"fix: retry card charges that time out",
	"chore: upgrade the payment client",
}
