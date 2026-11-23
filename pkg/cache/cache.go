// Package cache provides an in-process cache with TTL, JSON, sets and leases.
//
// The application is deployed as a single container and no longer depends on
// Redis. Only short-lived data (sessions, captcha, rate limits) is stored here;
// all durable data lives in SQLite.
package cache

import (
	"context"
	"encoding/json"
	"strconv"
	"sync"
	"time"
)

type item struct {
	value   string
	expires time.Time // zero means no expiry
}

func (i item) expired(now time.Time) bool {
	return !i.expires.IsZero() && now.After(i.expires)
}

// Cache is a namespaced in-process cache.
type Cache struct {
	prefix string

	mu    sync.RWMutex
	items map[string]item
	sets  map[string]map[string]struct{}
}

// New creates a memory cache.
func New(prefix string) *Cache {
	c := &Cache{
		prefix: prefix,
		items:  make(map[string]item),
		sets:   make(map[string]map[string]struct{}),
	}
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			c.cleanup()
		}
	}()
	return c
}

func (c *Cache) key(k string) string { return c.prefix + k }

func (c *Cache) cleanup() {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	for k, it := range c.items {
		if it.expired(now) {
			delete(c.items, k)
		}
	}
}

// Ping always succeeds for the memory cache.
func (c *Cache) Ping(context.Context) error { return nil }

// Get returns a string value.
func (c *Cache) Get(_ context.Context, k string) (string, bool) {
	c.mu.RLock()
	it, ok := c.items[c.key(k)]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if it.expired(time.Now()) {
		c.mu.Lock()
		delete(c.items, c.key(k))
		c.mu.Unlock()
		return "", false
	}
	return it.value, true
}

// Set stores a string value.
func (c *Cache) Set(_ context.Context, k, v string, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[c.key(k)] = item{value: v, expires: expiry(ttl)}
	return nil
}

// SetNX stores a value only when the key does not exist.
func (c *Cache) SetNX(_ context.Context, k, v string, ttl time.Duration) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.key(k)
	if it, ok := c.items[key]; ok && !it.expired(time.Now()) {
		return false, nil
	}
	c.items[key] = item{value: v, expires: expiry(ttl)}
	return true, nil
}

// Del removes keys.
func (c *Cache) Del(_ context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range keys {
		delete(c.items, c.key(k))
	}
	return nil
}

// Incr increments a counter and refreshes its TTL.
func (c *Cache) Incr(_ context.Context, k string, ttl time.Duration) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.key(k)
	now := time.Now()
	n := int64(0)
	if it, ok := c.items[key]; ok && !it.expired(now) {
		n, _ = strconv.ParseInt(it.value, 10, 64)
	}
	n++
	c.items[key] = item{value: strconv.FormatInt(n, 10), expires: expiry(ttl)}
	return n, nil
}

// GetInt reads an integer counter.
func (c *Cache) GetInt(ctx context.Context, k string) int64 {
	v, ok := c.Get(ctx, k)
	if !ok {
		return 0
	}
	n, _ := strconv.ParseInt(v, 10, 64)
	return n
}

// Exists reports whether a key exists.
func (c *Cache) Exists(ctx context.Context, k string) bool {
	_, ok := c.Get(ctx, k)
	return ok
}

// SetJSON stores a JSON value.
func (c *Cache) SetJSON(ctx context.Context, k string, v any, ttl time.Duration) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.Set(ctx, k, string(b), ttl)
}

// GetJSON reads a JSON value.
func (c *Cache) GetJSON(ctx context.Context, k string, out any) bool {
	s, ok := c.Get(ctx, k)
	if !ok {
		return false
	}
	return json.Unmarshal([]byte(s), out) == nil
}

// Remember loads a value on cache miss.
func (c *Cache) Remember(ctx context.Context, k string, ttl time.Duration, loader func() (string, error)) (string, error) {
	if v, ok := c.Get(ctx, k); ok {
		if v == emptyPlaceholder {
			return "", nil
		}
		return v, nil
	}
	v, err := loader()
	if err != nil {
		return "", err
	}
	if v == "" {
		_ = c.Set(ctx, k, emptyPlaceholder, 60*time.Second)
		return "", nil
	}
	_ = c.Set(ctx, k, v, jitter(ttl))
	return v, nil
}

const emptyPlaceholder = "\x00nil"

// AcquireLock obtains a lease.
func (c *Cache) AcquireLock(ctx context.Context, k, token string, ttl time.Duration) (bool, error) {
	return c.SetNX(ctx, k, token, ttl)
}

// ExtendLock renews a lease only for its current owner.
func (c *Cache) ExtendLock(_ context.Context, k, token string, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.key(k)
	it, ok := c.items[key]
	if !ok || it.expired(time.Now()) || it.value != token {
		return false
	}
	it.expires = expiry(ttl)
	c.items[key] = it
	return true
}

// ReleaseLock removes a lease only for its current owner.
func (c *Cache) ReleaseLock(_ context.Context, k, token string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.key(k)
	it, ok := c.items[key]
	if !ok || it.value != token {
		return false
	}
	delete(c.items, key)
	return true
}

// SetAdd adds members to a set.
func (c *Cache) SetAdd(_ context.Context, k string, members ...string) error {
	if len(members) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.key(k)
	set := c.sets[key]
	if set == nil {
		set = make(map[string]struct{}, len(members))
		c.sets[key] = set
	}
	for _, m := range members {
		set[m] = struct{}{}
	}
	return nil
}

// SetMembers returns a snapshot of set members.
func (c *Cache) SetMembers(_ context.Context, k string) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	set := c.sets[c.key(k)]
	out := make([]string, 0, len(set))
	for m := range set {
		out = append(out, m)
	}
	return out
}

// SetRemove removes members from a set.
func (c *Cache) SetRemove(_ context.Context, k string, members ...string) error {
	if len(members) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	key := c.key(k)
	set := c.sets[key]
	if set == nil {
		return nil
	}
	for _, m := range members {
		delete(set, m)
	}
	if len(set) == 0 {
		delete(c.sets, key)
	}
	return nil
}

func expiry(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(ttl)
}

func jitter(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return ttl
	}
	delta := int64(ttl) / 10
	if delta == 0 {
		return ttl
	}
	return ttl + time.Duration((int64(time.Now().UnixNano())%delta)-(delta/2))
}
