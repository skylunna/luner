package limiter

import (
	"sync"
	"time"
)

type Bucket struct {
	capacity   float64
	tokens     float64
	refillRate float64
	lastRefill time.Time
	mu         sync.Mutex
}

func NewBucket(capacity, refillRate float64) *Bucket {
	return &Bucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (b *Bucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(b.lastRefill).Seconds()
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now

	if b.tokens >= 1.0 {
		b.tokens -= 1.0
		return true
	}

	return false
}

type Manager struct {
	buckets map[string]*Bucket
	mu      sync.RWMutex
}

func NewManager() *Manager {
	return &Manager{
		buckets: make(map[string]*Bucket),
	}
}

func (m *Manager) GetBucket(name string) *Bucket {
	m.mu.RLock()
	b, ok := m.buckets[name]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return b
}

func (m *Manager) SetBucket(name string, b *Bucket) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.buckets[name] = b
}
