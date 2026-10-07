package agent

import (
	"bytes"
	"context"
	"log"
	"sync"
)

const flushAt = 20

type Reporter interface {
	Events(ctx context.Context, deploymentID string, lines []string) error
}

type lineSink struct {
	ctx          context.Context
	reporter     Reporter
	deploymentID string
	mu           sync.Mutex
	partial      []byte
	lines        []string
	tail         []string
}

func newSink(ctx context.Context, reporter Reporter, deploymentID string) *lineSink {
	return &lineSink{ctx: ctx, reporter: reporter, deploymentID: deploymentID}
}

func (s *lineSink) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.partial = append(s.partial, p...)
	for {
		i := bytes.IndexByte(s.partial, '\n')
		if i < 0 {
			break
		}
		s.add(string(s.partial[:i]))
		s.partial = s.partial[i+1:]
	}
	return len(p), nil
}

func (s *lineSink) add(line string) {
	s.lines = append(s.lines, line)
	s.tail = append(s.tail, line)
	if len(s.tail) > 5 {
		s.tail = s.tail[1:]
	}
	if len(s.lines) >= flushAt {
		s.flushLocked()
	}
}

func (s *lineSink) Flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.partial) > 0 {
		s.add(string(s.partial))
		s.partial = nil
	}
	s.flushLocked()
}

func (s *lineSink) flushLocked() {
	if len(s.lines) == 0 {
		return
	}
	if err := s.reporter.Events(s.ctx, s.deploymentID, s.lines); err != nil {
		log.Printf("could not report %d lines: %v", len(s.lines), err)
	}
	s.lines = nil
}

func (s *lineSink) Tail() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.tail...)
}
