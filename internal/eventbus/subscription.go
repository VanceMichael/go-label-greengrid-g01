package eventbus

import "github.com/VanceMichael/greengrid/internal/domain"

func registerSubscriber(b *Bus, id string, buffer int) (<-chan Event, func(), error) {
	if _, ok := b.subscribers[id]; ok {
		return nil, nil, domain.ErrAlreadyExists
	}
	s := &subscriber{id: id, ch: make(chan Event, buffer), done: make(chan struct{})}
	b.subscribers[id] = s
	return s.ch, func() { b.unsubscribe(id) }, nil
}
