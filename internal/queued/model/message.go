package model

import "time"

type Outgoing struct {
	Body           []byte
	ContentType    string
	IdempotencyKey string
	Due            time.Time
}

type Delivery struct {
	ID          string
	QueueID     string
	Body        []byte
	ContentType string
	PublishedAt time.Time
	Attempt     int
	Handle      any
}

type DeadReason struct {
	Status   int
	Error    string
	Attempts int
}

type DeadLetter struct {
	ID          string
	Attempts    int
	LastStatus  int
	LastError   string
	PublishedAt time.Time
	Body        []byte
}

type BackendStats struct {
	Depth       int64
	InFlight    int
	OldestAge   time.Duration
	DeadLetters int64
}
