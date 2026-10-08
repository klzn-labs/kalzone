package web

import (
	"errors"
	"fmt"
	"hash/fnv"
	"html/template"
	"strings"
	"time"

	"github.com/adrianozp/platform/pkg/clock"
)

func funcs() template.FuncMap {
	return template.FuncMap{
		"icon":     icon,
		"ago":      Ago,
		"date":     func(t time.Time) string { return t.UTC().Format("Jan 2, 2006 15:04 UTC") },
		"iso":      func(t time.Time) string { return t.UTC().Format(time.RFC3339) },
		"initials": Initials,
		"hue":      Hue,
		"add":      func(a, b int) int { return a + b },
		"dict":     dict,
		"upper":    strings.ToUpper,
		"join":     strings.Join,
		"short":    Short,
		"deref":    deref,
		"list":     func(values ...int) []int { return values },
		"digestOf": digestOf,
		"span":     Span,
	}
}

func Ago(t time.Time) string {
	d := clock.Now().Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute") + " ago"
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour") + " ago"
	case d < 30*24*time.Hour:
		return plural(int(d.Hours()/24), "day") + " ago"
	}
	return t.UTC().Format("Jan 2, 2006")
}

func plural(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

func Initials(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '-' || r == '_' })
	result := ""
	for _, w := range words {
		if len(result) < 2 {
			result += strings.ToUpper(w[:1])
		}
	}
	if result == "" {
		return "?"
	}
	return result
}

func Hue(key string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % 6)
}

func dict(values ...any) (map[string]any, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("dict needs key/value pairs")
	}
	result := make(map[string]any, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}
		result[key] = values[i+1]
	}
	return result, nil
}

func Short(value string, n int) string {
	value = strings.TrimPrefix(value, "sha256:")
	if len(value) <= n {
		return value
	}
	return value[:n]
}

func deref(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

func digestOf(ref string) string {
	_, digest, _ := strings.Cut(ref, "@")
	return digest
}

func Span(d time.Duration) string {
	text := d.String()
	if strings.HasSuffix(text, "m0s") {
		text = strings.TrimSuffix(text, "0s")
	}
	if strings.HasSuffix(text, "h0m") {
		text = strings.TrimSuffix(text, "0m")
	}
	return text
}
