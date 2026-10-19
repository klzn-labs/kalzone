package changelogs

import (
	"sync"

	"github.com/klzn-labs/kalzone/app/release/entities"
)

type flight struct {
	done      chan struct{}
	changelog entities.Changelog
	err       error
}

type flights struct {
	mu      sync.Mutex
	running map[string]*flight
}

func newFlights() *flights {
	return &flights{running: map[string]*flight{}}
}

func (f *flights) do(key string, fetch func() (entities.Changelog, error)) (entities.Changelog, error) {
	f.mu.Lock()
	if running, ok := f.running[key]; ok {
		f.mu.Unlock()
		<-running.done
		return running.changelog, running.err
	}
	current := &flight{done: make(chan struct{})}
	f.running[key] = current
	f.mu.Unlock()
	current.changelog, current.err = fetch()
	f.land(key, current)
	return current.changelog, current.err
}

func (f *flights) land(key string, current *flight) {
	f.mu.Lock()
	delete(f.running, key)
	f.mu.Unlock()
	close(current.done)
}
