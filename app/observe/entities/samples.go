package entities

import "time"

type Sample struct {
	At    time.Time
	Value float64
}
