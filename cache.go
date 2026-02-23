package main

import (
	"sync"
	"time"
)

type cacheEntry struct {
	data      *IPInfo
	expiresAt time.Time
}

type Cache struct {
	mu      sync.RWMutex
	items   map[string]*cacheEntry
	ttl     time.Duration
	stopCh  chan struct{}
}

func NewCache(ttl time.Duration) *Cache {
	c := &Cache{
		items:  make(map[string]*cacheEntry),
		ttl:    ttl,
		stopCh: make(chan struct{}),
	}
	go c.cleanup()
	return c
}

func (c *Cache) Get(ip string) (*IPInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.items[ip]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	result := *entry.data
	result.Cached = true
	return &result, true
}

func (c *Cache) Set(ip string, info *IPInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items[ip] = &cacheEntry{
		data:      info,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
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
