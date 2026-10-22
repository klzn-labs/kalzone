package entities

import (
	"fmt"
	"time"
)

var syntheticLines = []string{
	"GET /api/products?category=shoes 200 %dms",
	"GET /api/cart 200 %dms",
	"POST /api/cart/items 201 %dms",
	"GET /api/orders/recent 200 %dms",
	"POST /api/checkout 202 %dms",
	"GET /healthz 200 %dms",
}

func SyntheticLines(slug string, end time.Time, limit int) []LogLine {
	count := min(limit, 30)
	lines := make([]LogLine, 0, count)
	for i := range count {
		at := end.Add(-time.Duration(count-i) * 47 * time.Second)
		text := fmt.Sprintf(syntheticLines[i%len(syntheticLines)], 12+(i*37)%90)
		if i%11 == 7 {
			text = "warn: payment provider answered slowly, retrying once"
		}
		lines = append(lines, LogLine{At: at, Container: slug + "-green", Line: text})
	}
	return lines
}
