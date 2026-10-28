package seed

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
