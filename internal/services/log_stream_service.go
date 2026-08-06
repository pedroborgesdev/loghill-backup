package services

import (
	"sync"

	"logtheater/internal/domain"
)

type subscription struct {
	id uint64
	ch chan domain.LogEntry
}
type Hub struct {
	mu                 sync.RWMutex
	next               uint64
	maxClients, buffer int
	subs               map[string]map[uint64]chan domain.LogEntry
}

func NewHub(maxClients, buffer int) *Hub {
	return &Hub{maxClients: maxClients, buffer: buffer, subs: map[string]map[uint64]chan domain.LogEntry{}}
}
func (h *Hub) Subscribe(sender string) (<-chan domain.LogEntry, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	m := h.subs[sender]
	if m == nil {
		m = map[uint64]chan domain.LogEntry{}
		h.subs[sender] = m
	}
	if len(m) >= h.maxClients {
		return nil, nil, domain.ErrTooManySubscribers
	}
	h.next++
	id := h.next
	ch := make(chan domain.LogEntry, h.buffer)
	m[id] = ch
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if current, ok := h.subs[sender][id]; ok {
				delete(h.subs[sender], id)
				close(current)
			}
			if len(h.subs[sender]) == 0 {
				delete(h.subs, sender)
			}
		})
	}
	return ch, cancel, nil
}
func (h *Hub) Publish(sender string, e domain.LogEntry) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs[sender] {
		select {
		case ch <- e:
		default:
		}
	}
}
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.subs {
		for _, ch := range m {
			close(ch)
		}
	}
	h.subs = map[string]map[uint64]chan domain.LogEntry{}
}
