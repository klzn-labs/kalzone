package page

import (
	"encoding/base64"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/klzn-labs/kalzone/pkg/apperror"
)

const (
	DefaultLimit    = 20
	MaxLimit        = 100
	storedPrecision = time.Millisecond
)

var (
	errLimit = apperror.Invalid("limit must be a whole number from 1 to " + strconv.Itoa(MaxLimit)).WithField("limit").
			WithHint("leave limit out for " + strconv.Itoa(DefaultLimit) + " items, or send 1 to " + strconv.Itoa(MaxLimit))
	errCursor = apperror.Invalid("cursor is not valid").WithField("cursor").
			WithHint("send next_cursor from the previous page unchanged, or start again without cursor")
)

type Request struct {
	Limit int
	After string
}

type Page[T any] struct {
	Items []T
	Next  string
}

func Parse(limit, cursor string) (Request, error) {
	n, err := parseLimit(limit)
	if err != nil {
		return Request{}, err
	}
	after, err := decode(cursor)
	return Request{Limit: n, After: after}, err
}

func First(limit int) Request {
	return Request{Limit: limit}
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return DefaultLimit, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > MaxLimit {
		return 0, errLimit
	}
	return n, nil
}

func decode(cursor string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil || len(raw) == 0 {
		return "", errCursor
	}
	return string(raw), nil
}

func Encode(key string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(key))
}

func (r Request) Fetch() int {
	return r.size() + 1
}

func (r Request) size() int {
	if r.Limit < 1 {
		return DefaultLimit
	}
	return r.Limit
}

func (r Request) Number() (int, error) {
	if r.After == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(r.After)
	if err != nil || n < 0 {
		return 0, errCursor
	}
	return n, nil
}

func TimeKey(at time.Time, id string) string {
	return at.UTC().Round(storedPrecision).Format(time.RFC3339Nano) + "|" + id
}

func (r Request) Time() (*time.Time, string, error) {
	if r.After == "" {
		return nil, "", nil
	}
	raw, id, found := strings.Cut(r.After, "|")
	at, err := time.Parse(time.RFC3339Nano, raw)
	if !found || err != nil {
		return nil, "", errCursor
	}
	return &at, id, nil
}

func Cut[T any](items []T, r Request, key func(T) string) Page[T] {
	if len(items) <= r.size() {
		return Page[T]{Items: items}
	}
	kept := items[:r.size()]
	return Page[T]{Items: kept, Next: Encode(key(kept[len(kept)-1]))}
}

func Window[T any](all []T, r Request, key func(T) string) (Page[T], error) {
	start, err := startOf(all, r.After, key)
	if err != nil {
		return Page[T]{}, err
	}
	end := min(start+r.Fetch(), len(all))
	return Cut(all[start:end], r, key), nil
}

func startOf[T any](all []T, after string, key func(T) string) (int, error) {
	if after == "" {
		return 0, nil
	}
	i := slices.IndexFunc(all, func(item T) bool { return key(item) == after })
	if i < 0 {
		return 0, errCursor
	}
	return i + 1, nil
}
