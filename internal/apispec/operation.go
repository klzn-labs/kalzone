package apispec

import (
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/klzn-labs/kalzone/pkg/approval"
	"github.com/klzn-labs/kalzone/pkg/page"
	"github.com/klzn-labs/kalzone/pkg/scope"
)

const BasePath = "/api/v1"

type Operation struct {
	ID       string
	Method   string
	Path     string
	Summary  string
	Scope    scope.Scope
	Session  bool
	Gated    bool
	Waits    bool
	Params   any
	Body     any
	BodyType string
	Status   int
	Response any
	Others   []Answer
	Handler  gin.HandlerFunc
}

type Answer struct {
	Status   int
	Response any
}

type Catalog struct {
	operations []Operation
}

func NewCatalog() *Catalog {
	return &Catalog{}
}

func (c *Catalog) Add(op Operation) {
	c.operations = append(c.operations, op)
}

func (c *Catalog) Operations() []Operation {
	return slices.Clone(c.operations)
}

type OrChangeRequest[T any] struct{}

func (OrChangeRequest[T]) JSONSchemaOneOf() []any {
	var answer T
	return []any{answer, approval.Pending{}}
}

type BatchOrChangeRequest[T any] struct{}

func (BatchOrChangeRequest[T]) JSONSchemaOneOf() []any {
	var one T
	return []any{one, Page[T]{}, approval.Pending{}}
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func Items[E, D any](p page.Page[E], convert func(E) D) Page[D] {
	items := make([]D, 0, len(p.Items))
	for _, item := range p.Items {
		items = append(items, convert(item))
	}
	return Page[D]{Items: items, NextCursor: p.Next}
}

func Window[E, D any](all []E, r page.Request, key func(E) string, convert func(E) D) (Page[D], error) {
	window, err := page.Window(all, r, key)
	return Items(window, convert), err
}
