// Package cache stores HTTP response bodies as files keyed by sha256(url).
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Cache is a directory of JSON entries. A zero TTL means the entry never expires.
type Cache struct {
	Dir string
	Now func() time.Time
}

type entry struct {
	URL       string          `json:"url"`
	FetchedAt time.Time       `json:"fetched_at"`
	TTLSec    int64           `json:"ttl_s"`
	Status    int             `json:"status"`
	Body      json.RawMessage `json:"body"`
}

// New creates the cache directory if needed.
func New(dir string) (*Cache, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Cache{Dir: dir, Now: time.Now}, nil
}

func (c *Cache) path(url string) string {
	h := sha256.Sum256([]byte(url))
	k := hex.EncodeToString(h[:])
	return filepath.Join(c.Dir, k[:2], k+".json")
}

// Get returns the cached status and body for url if present and fresh.
func (c *Cache) Get(url string) (status int, body []byte, ok bool) {
	b, err := os.ReadFile(c.path(url))
	if err != nil {
		return 0, nil, false
	}
	var e entry
	if json.Unmarshal(b, &e) != nil || e.URL != url {
		return 0, nil, false
	}
	if e.TTLSec > 0 && c.Now().Sub(e.FetchedAt) > time.Duration(e.TTLSec)*time.Second {
		return 0, nil, false
	}
	return e.Status, e.Body, true
}

// Put stores a JSON body (status 200) or an empty body (e.g. 404 = zero views).
func (c *Cache) Put(url string, status int, body []byte, ttl time.Duration) error {
	if len(body) == 0 || !json.Valid(body) {
		body = []byte("null")
	}
	e := entry{URL: url, FetchedAt: c.Now(), TTLSec: int64(ttl / time.Second), Status: status, Body: body}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	p := c.path(url)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
