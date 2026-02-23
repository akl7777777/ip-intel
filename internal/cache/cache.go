package cache

import (
	"sync"
	"time"

	"github.com/akl7777777/ip-intel/internal/model"
)

type entry struct {
	data      *model.IPInfo
	expiresAt time.Time
}

type Cache struct {
	mu     sync.RWMutex
	items  map[string]*entry
	ttl    time.Duration
	stopCh chan struct{}
}

func New(ttl time.Duration) *Cache {
	c := &Cache{
		items:  make(map[string]*entry),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	go c.cleanup()
	return c
}

func (c *Cache) Get(ip string) (*model.IPInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.items[ip]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		return nil, false
	}
	result := *e.data
	result.Cached = true
	return &result, true
}

func (c *Cache) Set(ip string, info *model.IPInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[ip] = &entry{
		data:      info,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

func (c *Cache) TTL() time.Duration {
	return c.ttl
}

func (c *Cache) Stop() {
	close(c.stopCh)
}

func (c *Cache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for k, v := range c.items {
				if now.After(v.expiresAt) {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		case <-c.stopCh:
			return
		}
	}
}
