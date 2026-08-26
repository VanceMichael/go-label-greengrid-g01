package eventbus

import (
	"context"
	"sync"

	"github.com/VanceMichael/greengrid/internal/domain"
)

type Event struct {
	Kind, TenantID, AggregateID string
	Payload                     any
}

type subscriber struct {
	id   string
	ch   chan Event
	done chan struct{}
	once sync.Once
}

type Bus struct {
	mu          sync.RWMutex
	subscribers map[string]*subscriber
	closed      bool
}

func New() *Bus { return &Bus{subscribers: make(map[string]*subscriber)} }

func (b *Bus) Subscribe(id string, buffer int) (<-chan Event, func(), error) {
	if id == "" || buffer < 1 {
		return nil, nil, domain.ErrInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, nil, domain.ErrState
	}
	if _, ok := b.subscribers[id]; ok {
		return nil, nil, domain.ErrAlreadyExists
	}
	s := &subscriber{id: id, ch: make(chan Event, buffer), done: make(chan struct{})}
	b.subscribers[id] = s
	return s.ch, func() { b.unsubscribe(id) }, nil
}

func (b *Bus) unsubscribe(id string) {
	b.mu.Lock()
	s, ok := b.subscribers[id]
	if ok {
		delete(b.subscribers, id)
	}
	b.mu.Unlock()
	if ok {
		s.once.Do(func() { close(s.done); close(s.ch) })
	}
}

func (b *Bus) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	list := make([]*subscriber, 0, len(b.subscribers))
	for _, s := range b.subscribers {
		list = append(list, s)
	}
	b.mu.RUnlock()
	for _, s := range list {
		select {
		case s.ch <- event:
		case <-s.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (b *Bus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	ids := make([]string, 0, len(b.subscribers))
	for id := range b.subscribers {
		ids = append(ids, id)
	}
	b.mu.Unlock()
	for _, id := range ids {
		b.unsubscribe(id)
	}
}

func (b *Bus) SubscriberCount() int { b.mu.RLock(); defer b.mu.RUnlock(); return len(b.subscribers) }
