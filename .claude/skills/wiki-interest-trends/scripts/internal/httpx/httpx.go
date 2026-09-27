// Package httpx is the only way the skill talks to the network:
// explicit User-Agent, one shared rate limiter, retries on 429/5xx, file cache.
package httpx

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"wikitrend/internal/cache"
	"wikitrend/internal/out"
)

// Version is reported in the User-Agent.
const Version = "0.1.0"

// UserAgent follows the Wikimedia policy: <client>/<version> (<contact>) <library>.
func UserAgent(contact string) string {
	return fmt.Sprintf("wikitrend/%s (%s) go-net-http", Version, contact)
}

// Client performs cached, rate-limited GET requests.
type Client struct {
	HTTP      *http.Client
	Cache     *cache.Cache
	UA        string
	Interval  time.Duration // minimum gap between network requests
	MaxTries  int
	Requests  int // network requests made (for stats)
	CacheHits int

	mu   sync.Mutex
	last time.Time
}

// New returns a client that makes at most ~3 requests/s (Wikimedia allows 200/min with a proper UA).
func New(c *cache.Cache, contact string) *Client {
	return &Client{
		HTTP:     &http.Client{Timeout: 30 * time.Second},
		Cache:    c,
		UA:       UserAgent(contact),
		Interval: 350 * time.Millisecond,
		MaxTries: 4,
	}
}

func (c *Client) wait() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d := c.Interval - time.Since(c.last); d > 0 {
		time.Sleep(d)
	}
	c.last = time.Now()
}

// Get returns (status, body). 404 is returned as a normal status, not an error.
// ttl == 0 caches forever.
func (c *Client) Get(url string, ttl time.Duration) (int, []byte, error) {
	if c.Cache != nil {
		if st, b, ok := c.Cache.Get(url); ok {
			c.CacheHits++
			return st, b, nil
		}
	}
	var lastErr error
	backoff := time.Second
	for try := 1; try <= c.MaxTries; try++ {
		c.wait()
		c.Requests++
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("User-Agent", c.UA)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = out.Errf("NETWORK", "Check the internet connection, then rerun the same command.", "request failed: %v", err)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		switch {
		case resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotFound:
			if c.Cache != nil {
				_ = c.Cache.Put(url, resp.StatusCode, body, ttl)
			}
			return resp.StatusCode, body, nil
		case resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500:
			wait := backoff
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
				wait = time.Duration(s) * time.Second
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				e := out.Errf("RATE_LIMITED", "Wait retry_after_s seconds, then rerun the same command (already downloaded data is cached).", "Wikimedia returned 429 for %s", url)
				e.RetryAfterSec = int(wait / time.Second)
				lastErr = e
			} else {
				lastErr = out.Errf("UPSTREAM", "Wait a minute, then rerun the same command.", "Wikimedia returned %d", resp.StatusCode)
			}
			if wait > 65*time.Second { // longer waits go back to the agent as RATE_LIMITED
				return 0, nil, lastErr
			}
			time.Sleep(wait)
			backoff *= 2
		default:
			return resp.StatusCode, body, out.Errf("UPSTREAM", "Check the arguments; this request is not retryable.", "HTTP %d for %s: %.200s", resp.StatusCode, url, body)
		}
	}
	return 0, nil, lastErr
}
