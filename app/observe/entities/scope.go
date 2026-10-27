package entities

import (
	"regexp"
	"strings"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

const outOfScopeLabel = "kalzone_out_of_scope"

type Scope struct {
	Services     []string
	Environments []string
	MySQLUsers   []string
	Schemas      []string
	Queues       []string
}

type scopeRule struct {
	prefix string
	limit  func(Scope) *labels.Matcher
}

var scopeRules = []scopeRule{
	{prefix: "mysql_global_"},
	{prefix: "mysql_info_schema_processlist_", limit: func(s Scope) *labels.Matcher { return oneOf("mysql_user", s.MySQLUsers) }},
	{prefix: "mysql_perf_schema_events_statements", limit: func(s Scope) *labels.Matcher { return oneOf("schema", s.Schemas) }},
	{prefix: "traefik_service_", limit: func(s Scope) *labels.Matcher { return s.colours("service", "@file") }},
	{prefix: "container_", limit: func(s Scope) *labels.Matcher { return s.colours("name", "") }},
	{prefix: "kalzone_traffic_weight_percent", limit: func(s Scope) *labels.Matcher { return s.deployed("service") }},
	{prefix: "kalzone_database_attachment", limit: func(s Scope) *labels.Matcher { return oneOf("service", s.Services) }},
	{prefix: "platform_queue_", limit: func(s Scope) *labels.Matcher { return oneOf("queue_id", s.Queues) }},
}

var promql = parser.NewParser(parser.Options{})

func (s Scope) Tenants() []string {
	tenants := []string{}
	for _, service := range s.Services {
		for _, env := range s.Environments {
			tenants = append(tenants, service+"-"+env)
		}
	}
	return tenants
}

func ScopeQuery(query string, s Scope) (string, error) {
	expr, err := promql.ParseExpr(query)
	if err != nil {
		return "", err
	}
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		if selector, ok := node.(*parser.VectorSelector); ok {
			selector.LabelMatchers = append(selector.LabelMatchers, s.limitFor(selector.LabelMatchers)...)
		}
		return nil
	})
	return expr.String(), nil
}

func ScopeSelector(selector string, s Scope) (string, error) {
	matchers, err := promql.ParseMetricSelector(selector)
	if err != nil {
		return "", err
	}
	return render(append(matchers, s.limitFor(matchers)...)), nil
}

func ScopeDefaultSelectors(s Scope) []string {
	selectors := make([]string, 0, len(scopeRules))
	for _, rule := range scopeRules {
		family := labels.MustNewMatcher(labels.MatchRegexp, model.MetricNameLabel, regexp.QuoteMeta(rule.prefix)+".*")
		selectors = append(selectors, render(append([]*labels.Matcher{family}, limitOf(rule, s)...)))
	}
	return selectors
}

func (s Scope) limitFor(matchers []*labels.Matcher) []*labels.Matcher {
	if pinsSharedDatabaseContainer(matchers) {
		return nil
	}
	name := metricName(matchers)
	for _, rule := range scopeRules {
		if name != "" && strings.HasPrefix(name, rule.prefix) {
			return limitOf(rule, s)
		}
	}
	return []*labels.Matcher{outOfScope()}
}

func limitOf(rule scopeRule, s Scope) []*labels.Matcher {
	if rule.limit == nil {
		return nil
	}
	return []*labels.Matcher{rule.limit(s)}
}

func metricName(matchers []*labels.Matcher) string {
	for _, m := range matchers {
		if m.Name == model.MetricNameLabel && m.Type == labels.MatchEqual {
			return m.Value
		}
	}
	return ""
}

func pinsSharedDatabaseContainer(matchers []*labels.Matcher) bool {
	for _, m := range matchers {
		if m.Name == "name" && m.Type == labels.MatchEqual && m.Value == mysqlContainer {
			return strings.HasPrefix(metricName(matchers), "container_")
		}
	}
	return false
}

func (s Scope) colours(label, suffix string) *labels.Matcher {
	return s.matchDeployed(label, "-(?:blue|green)"+regexp.QuoteMeta(suffix))
}

func (s Scope) deployed(label string) *labels.Matcher {
	return s.matchDeployed(label, "")
}

func (s Scope) matchDeployed(label, tail string) *labels.Matcher {
	if len(s.Services) == 0 || len(s.Environments) == 0 {
		return outOfScope()
	}
	return labels.MustNewMatcher(labels.MatchRegexp, label, alternatives(s.Services)+"-"+alternatives(s.Environments)+tail)
}

func oneOf(label string, values []string) *labels.Matcher {
	if len(values) == 0 {
		return outOfScope()
	}
	return labels.MustNewMatcher(labels.MatchRegexp, label, alternatives(values))
}

func alternatives(values []string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = regexp.QuoteMeta(v)
	}
	return "(?:" + strings.Join(quoted, "|") + ")"
}

func outOfScope() *labels.Matcher {
	return labels.MustNewMatcher(labels.MatchEqual, outOfScopeLabel, "true")
}

func render(matchers []*labels.Matcher) string {
	return (&parser.VectorSelector{LabelMatchers: matchers}).String()
}
