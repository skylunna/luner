package cache

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

type Item struct {
	Key      string
	Value    []byte
	ExpireAt time.Time
}

type LRU struct {
	capacity int
	ttl      time.Duration
	items    map[string]*list.Element
	ll       *list.List
	mu       sync.RWMutex
}

func NewLRU(capacity int, ttl time.Duration) *LRU {
	return &LRU{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*list.Element),
		ll:       list.New(),
	}
}

func (c *LRU) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		item := elem.Value.(*Item)
		if time.Now().Before(item.ExpireAt) {
			c.ll.MoveToFront(elem)
			return item.Value, true
		}
		c.removeElement(elem)
	}
	return nil, false
}

func (c *LRU) Set(key string, value []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.ll.MoveToFront(elem)
		item := elem.Value.(*Item)
		item.Value = value
		item.ExpireAt = time.Now().Add(c.ttl)
		return
	}

	if c.ll.Len() >= c.capacity {
		c.removeOldest()
	}

	item := &Item{Key: key, Value: value, ExpireAt: time.Now().Add(c.ttl)}
	elem := c.ll.PushFront(item)
	c.items[key] = elem
}

func (c *LRU) removeOldest() {
	if elem := c.ll.Back(); elem != nil {
		c.removeElement(elem)
	}
}

func (c *LRU) removeElement(e *list.Element) {
	c.ll.Remove(e)
	item := e.Value.(*Item)
	delete(c.items, item.Key)
}

// 基于 model + messages 哈希 + temperature 生成缓存 key
func GenerateKey(model string, messagesJSON []byte, temp float64) string {
	h := sha256.Sum256(messagesJSON)
	return fmt.Sprintf("%s:%s:%.1f", model, hex.EncodeToString(h[:8]), temp)
}
