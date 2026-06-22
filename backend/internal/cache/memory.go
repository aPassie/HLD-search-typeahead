package cache

import (
	"container/list"
	"sync"
	"time"

	"searchtypeahead/internal/model"
)

type entry struct {
	key       string
	val       []model.Candidate
	expiresAt time.Time
}

// Memory is an in-process LRU + TTL cache node. Each node has its own mutex, so
// the ring shards lock contention across nodes.
type Memory struct {
	id      string
	maxKeys int
	mu      sync.Mutex
	ll      *list.List // front = most recently used
	items   map[string]*list.Element
}

// NewMemory creates an in-process node holding up to maxKeys entries.
func NewMemory(id string, maxKeys int) *Memory {
	if maxKeys < 1 {
		maxKeys = 1024
	}
	return &Memory{id: id, maxKeys: maxKeys, ll: list.New(), items: map[string]*list.Element{}}
}

func (m *Memory) ID() string { return m.id }

func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ll.Len()
}

// Get returns the value and moves the entry to the front. TTL is checked on
// every lookup (lazy expiry); an expired entry is evicted and reported as a miss.
func (m *Memory) Get(key string) ([]model.Candidate, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	el, ok := m.items[key]
	if !ok {
		return nil, false
	}
	e := el.Value.(*entry)
	if time.Now().After(e.expiresAt) {
		m.removeElement(el)
		return nil, false
	}
	m.ll.MoveToFront(el)
	return e.val, true
}

// Set inserts or updates the entry, evicting the least-recently-used while over capacity.
func (m *Memory) Set(key string, val []model.Candidate, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	exp := time.Now().Add(ttl)
	if el, ok := m.items[key]; ok {
		e := el.Value.(*entry)
		e.val, e.expiresAt = val, exp
		m.ll.MoveToFront(el)
		return
	}
	m.items[key] = m.ll.PushFront(&entry{key: key, val: val, expiresAt: exp})
	for m.ll.Len() > m.maxKeys {
		m.removeElement(m.ll.Back())
	}
}

// Peek reports presence/expiry without touching LRU order or evicting.
func (m *Memory) Peek(key string) State {
	m.mu.Lock()
	defer m.mu.Unlock()
	el, ok := m.items[key]
	if !ok {
		return StateMiss
	}
	if time.Now().After(el.Value.(*entry).expiresAt) {
		return StateExpired
	}
	return StateHit
}

// Delete removes a key if present.
func (m *Memory) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if el, ok := m.items[key]; ok {
		m.removeElement(el)
	}
}

func (m *Memory) removeElement(el *list.Element) {
	if el == nil {
		return
	}
	m.ll.Remove(el)
	delete(m.items, el.Value.(*entry).key)
}
