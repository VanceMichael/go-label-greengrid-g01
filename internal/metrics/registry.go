package metrics

import (
	"sync"
	"sync/atomic"
)

type Counter struct{ value atomic.Int64 }

func (c *Counter) Add(n int64)  { c.value.Add(n) }
func (c *Counter) Inc()         { c.Add(1) }
func (c *Counter) Value() int64 { return c.value.Load() }

type Registry struct {
	mu       sync.RWMutex
	counters map[string]*Counter
}

func New() *Registry { return &Registry{counters: make(map[string]*Counter)} }
func (r *Registry) Counter(name string) *Counter {
	r.mu.RLock()
	c := r.counters[name]
	r.mu.RUnlock()
	if c != nil {
		return c
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c = r.counters[name]; c == nil {
		c = &Counter{}
		r.counters[name] = c
	}
	return c
}
func (r *Registry) Snapshot() map[string]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]int64, len(r.counters))
	for k, v := range r.counters {
		out[k] = v.Value()
	}
	return out
}
