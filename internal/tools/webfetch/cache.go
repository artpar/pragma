package webfetch

import (
	"sync"
	"time"
)

type cacheEntry struct {
	content   string
	size      int64
	expiresAt time.Time
}

// urlCache is a simple TTL + size-bounded URL content cache.
type urlCache struct {
	mu      sync.Mutex
	entries map[string]*cacheEntry
	size    int64
	maxSize int64
	ttl     time.Duration
}

func newURLCache(maxSize int64, ttl time.Duration) *urlCache {
	return &urlCache{
		entries: make(map[string]*cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get returns cached content for a URL, or ("", false) if not found/expired.
func (c *urlCache) Get(url string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[url]
	if !ok {
		return "", false
	}
	if time.Now().After(entry.expiresAt) {
		c.size -= entry.size
		delete(c.entries, url)
		return "", false
	}
	return entry.content, true
}

// Set stores content in the cache. Evicts oldest entries if size limit is exceeded.
func (c *urlCache) Set(url, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entrySize := int64(len(content))

	// Remove existing entry for this URL if present
	if existing, ok := c.entries[url]; ok {
		c.size -= existing.size
		delete(c.entries, url)
	}

	// Evict expired entries first
	now := time.Now()
	for k, v := range c.entries {
		if now.After(v.expiresAt) {
			c.size -= v.size
			delete(c.entries, k)
		}
	}

	// If still over limit, evict oldest entries
	for c.size+entrySize > c.maxSize && len(c.entries) > 0 {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldestKey == "" || v.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiresAt
			}
		}
		c.size -= c.entries[oldestKey].size
		delete(c.entries, oldestKey)
	}

	c.entries[url] = &cacheEntry{
		content:   content,
		size:      entrySize,
		expiresAt: now.Add(c.ttl),
	}
	c.size += entrySize
}
