package hf

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Client talks to huggingface.co and the dataset-viewer service. It is safe for
// concurrent use: the pacer, the cache, and the namespace memo are all
// synchronised, so a crawl can run many workers through one client and still
// send a polite, single stream of requests.
type Client struct {
	HTTP      *http.Client
	Base      string // the hub, https://huggingface.co
	Viewer    string // the dataset viewer, https://datasets-server.huggingface.co
	Token     string // a hub token, or empty for anonymous reads
	UserAgent string

	// Rate is the minimum gap between requests, shared across workers. Zero
	// disables pacing.
	Rate    time.Duration
	Retries int

	// CacheDir is where responses are stored. Empty disables the cache.
	CacheDir string
	NoCache  bool
	CacheTTL time.Duration

	// Deep makes record fetches also read the rendered page and merge the
	// fields only the page carries. Card includes README text.
	Deep bool
	Card bool

	// Workers is the fan-out for commands that fetch many things.
	Workers int

	mu     sync.Mutex
	last   time.Time
	nsKind sync.Map // namespace name -> "user" or "org"
	tagsMu sync.Mutex
	tags   *Taxonomy
}

// NewClient returns a client with the defaults from the spec: a 30 second
// timeout, a 150ms gap between requests, five retries, and eight workers.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		Base:      BaseURL,
		Viewer:    ViewerURL,
		UserAgent: DefaultUserAgent,
		Rate:      150 * time.Millisecond,
		Retries:   5,
		CacheTTL:  15 * time.Minute,
		Workers:   8,
		Token:     TokenFromEnv(),
	}
}

// TokenFromEnv finds a hub token the way the official tools do: the two
// environment variables first, then the file the huggingface-cli login writes.
// An explicit --token beats all of them and is applied by the caller.
func TokenFromEnv() string {
	for _, k := range []string{"HF_TOKEN", "HUGGING_FACE_HUB_TOKEN"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	home := os.Getenv("HF_HOME")
	if home == "" {
		if h, err := os.UserHomeDir(); err == nil {
			home = filepath.Join(h, ".cache", "huggingface")
		}
	}
	if home == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(home, "token"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// Response is one completed request: the body plus the parts of the exchange a
// caller needs. FinalURL differs from the requested URL when the hub redirected
// a legacy id to its canonical owner, which is how AliasOf gets filled.
type Response struct {
	Body     []byte
	Status   int
	Header   http.Header
	URL      string
	FinalURL string
}

// Get fetches a URL and returns the response. Every request in the package goes
// through here, so pacing, auth, caching, retry, and error classification all
// have exactly one home.
func (c *Client) Get(ctx context.Context, rawURL string) (*Response, error) {
	return c.request(ctx, http.MethodGet, rawURL, nil, "")
}

// Stream fetches a URL and hands back the open body. It is the one route that
// does not buffer, retry, or cache, because the files it is for are model
// weights and a copy of one of those does not belong in memory or in a cache
// directory. The caller closes the reader.
func (c *Client) Stream(ctx context.Context, rawURL string) (io.ReadCloser, http.Header, error) {
	c.pace(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "*/*")
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, wrapNetwork(rawURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The body of an error is small and is the only place the hub explains
		// itself, so it is worth reading before giving up on the response.
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
		_ = resp.Body.Close()
		return nil, nil, c.withRetryAfter(statusError(rawURL, resp.StatusCode, b), resp)
	}
	return resp.Body, resp.Header, nil
}

// PostJSON sends a JSON body. Only paths-info needs it, but the hub's read API
// does use POST in that one place.
func (c *Client) PostJSON(ctx context.Context, rawURL string, payload any) (*Response, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return c.request(ctx, http.MethodPost, rawURL, b, "application/json")
}

// GetJSON fetches and decodes in one step.
func (c *Client) GetJSON(ctx context.Context, rawURL string, v any) (*Response, error) {
	resp, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	if v != nil {
		if err := json.Unmarshal(resp.Body, v); err != nil {
			return resp, fmt.Errorf("decode %s: %w", rawURL, err)
		}
	}
	return resp, nil
}

func (c *Client) request(ctx context.Context, method, rawURL string, body []byte, contentType string) (*Response, error) {
	if method == http.MethodGet {
		if hit, ok := c.cacheGet(rawURL); ok {
			return hit, nil
		}
	}
	var last error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			d := backoff(attempt, last)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(d):
			}
		}
		resp, status, retry, err := c.do(ctx, method, rawURL, body, contentType, false)
		if err == nil {
			if method == http.MethodGet {
				c.cachePut(rawURL, resp)
			}
			return resp, nil
		}
		// A fine-grained token can be narrower than no token at all. The hub
		// answers some public routes anonymously and refuses the same routes when
		// a token without the matching scope is attached, so a refusal is worth
		// one anonymous retry before it becomes the answer. This never sends the
		// token anywhere it was not already sent.
		if (status == http.StatusUnauthorized || status == http.StatusForbidden) && c.Token != "" {
			if resp, _, _, aerr := c.do(ctx, method, rawURL, body, contentType, true); aerr == nil {
				if method == http.MethodGet {
					c.cachePut(rawURL, resp)
				}
				return resp, nil
			}
		}
		last = err
		if !retry {
			return nil, err
		}
	}
	return nil, last
}

func (c *Client) do(ctx context.Context, method, rawURL string, body []byte, contentType string, anon bool) (*Response, int, bool, error) {
	c.pace(ctx)

	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rdr)
	if err != nil {
		return nil, 0, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json, text/html;q=0.9, */*;q=0.8")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.Token != "" && !anon {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, true, wrapNetwork(rawURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, true, wrapNetwork(rawURL, err)
	}
	out := &Response{Body: b, Status: resp.StatusCode, Header: resp.Header, URL: rawURL}
	if resp.Request != nil && resp.Request.URL != nil {
		out.FinalURL = resp.Request.URL.String()
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return out, resp.StatusCode, false, nil
	}
	err = statusError(rawURL, resp.StatusCode, b)
	return nil, resp.StatusCode, retryable(resp.StatusCode), c.withRetryAfter(err, resp)
}

// withRetryAfter attaches the server's own backoff hint, so a 429 waits exactly
// as long as it was asked to rather than guessing.
func (c *Client) withRetryAfter(err error, resp *http.Response) error {
	v := resp.Header.Get("Retry-After")
	if v == "" {
		return err
	}
	if d, perr := time.ParseDuration(v + "s"); perr == nil {
		return &retryAfterError{err: err, after: d}
	}
	if t, perr := http.ParseTime(v); perr == nil {
		return &retryAfterError{err: err, after: time.Until(t)}
	}
	return err
}

type retryAfterError struct {
	err   error
	after time.Duration
}

func (e *retryAfterError) Error() string { return e.err.Error() }
func (e *retryAfterError) Unwrap() error { return e.err }

// pace blocks until the client's minimum gap has passed since the last request.
// It holds the lock across the sleep on purpose: the point is that N workers
// produce one paced stream, not N paced streams.
func (c *Client) pace(ctx context.Context) {
	if c.Rate <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
	c.last = time.Now()
}

func retryable(status int) bool {
	return status == http.StatusTooManyRequests || status >= 500
}

// backoff is exponential with full jitter, unless the server sent a Retry-After,
// in which case it said what it wanted and we do that.
func backoff(attempt int, last error) time.Duration {
	var ra *retryAfterError
	if errorsAs(last, &ra) && ra.after > 0 {
		if ra.after > 5*time.Minute {
			return 5 * time.Minute
		}
		return ra.after
	}
	d := time.Duration(1<<uint(attempt-1)) * 500 * time.Millisecond
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d/2 + time.Duration(jitter(int64(d/2)))
}

// --- URL building ---

// api builds a hub API URL: api("models", id) is /api/models/<id>.
func (c *Client) api(parts ...string) string {
	return c.Base + "/api/" + strings.Join(parts, "/")
}

// query appends parameters, skipping empty values so a caller can pass optional
// filters straight through.
func query(base string, kv ...string) string {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i+1] != "" {
			v.Add(kv[i], kv[i+1])
		}
	}
	if len(v) == 0 {
		return base
	}
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return base + sep + v.Encode()
}

// --- pagination ---

// nextLink reads the cursor out of the Link header. The cursor is opaque and
// must be passed back verbatim: constructing or decoding it would break the
// moment the hub changes its encoding.
func nextLink(h http.Header) string {
	for _, link := range h.Values("Link") {
		for _, part := range strings.Split(link, ",") {
			part = strings.TrimSpace(part)
			if !strings.Contains(part, `rel="next"`) {
				continue
			}
			if i := strings.Index(part, "<"); i >= 0 {
				if j := strings.Index(part[i:], ">"); j > 0 {
					return part[i+1 : i+j]
				}
			}
		}
	}
	return ""
}

// errStopPage ends a page walk early. A command with -n returns it so the next
// page is never requested, which is the difference between a cheap head and an
// accidental full crawl.
var errStopPage = fmt.Errorf("stop paging")

// eachPage walks a cursor-paginated list, calling fn once per page of raw
// elements. It stops when there is no next link, not when a page comes back
// short, because short pages happen.
func (c *Client) eachPage(ctx context.Context, rawURL string, fn func([]json.RawMessage, *Response) error) error {
	for rawURL != "" {
		resp, err := c.Get(ctx, rawURL)
		if err != nil {
			return err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(resp.Body, &items); err != nil {
			return fmt.Errorf("decode %s: %w", rawURL, err)
		}
		if err := fn(items, resp); err != nil {
			if err == errStopPage {
				return nil
			}
			return err
		}
		rawURL = nextLink(resp.Header)
	}
	return nil
}

// --- the on-disk response cache ---

func (c *Client) cacheKey(rawURL string) string {
	auth := "anon"
	if c.Token != "" {
		auth = "auth"
	}
	sum := sha256.Sum256([]byte(auth + " " + rawURL))
	return hex.EncodeToString(sum[:])
}

func (c *Client) cacheGet(rawURL string) (*Response, bool) {
	if c.NoCache || c.CacheDir == "" {
		return nil, false
	}
	key := c.cacheKey(rawURL)
	path := filepath.Join(c.CacheDir, key[:2], key)
	st, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	// A response at an immutable revision never changes, so it never expires.
	if ttl := c.ttlFor(rawURL); ttl > 0 && time.Since(st.ModTime()) > ttl {
		return nil, false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var ent cacheEntry
	if err := json.Unmarshal(b, &ent); err != nil {
		return nil, false
	}
	return &Response{Body: ent.Body, Status: ent.Status, Header: ent.Header, URL: rawURL, FinalURL: ent.FinalURL}, true
}

func (c *Client) cachePut(rawURL string, resp *Response) {
	if c.CacheDir == "" || resp == nil {
		return
	}
	key := c.cacheKey(rawURL)
	dir := filepath.Join(c.CacheDir, key[:2])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	b, err := json.Marshal(cacheEntry{Body: resp.Body, Status: resp.Status, Header: resp.Header, FinalURL: resp.FinalURL})
	if err != nil {
		return
	}
	// Write through a temp file so a killed process cannot leave a half entry
	// that later decodes as valid.
	tmp := filepath.Join(dir, key+".tmp")
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, filepath.Join(dir, key))
}

type cacheEntry struct {
	Body     []byte      `json:"body"`
	Status   int         `json:"status"`
	Header   http.Header `json:"header,omitempty"`
	FinalURL string      `json:"finalUrl,omitempty"`
}

// ttlFor gives long-lived documents a long life. The taxonomy and the task
// pages change on the order of weeks; anything pinned to a commit sha never
// changes at all.
func (c *Client) ttlFor(rawURL string) time.Duration {
	switch {
	case strings.Contains(rawURL, "-tags-by-type") || strings.Contains(rawURL, "/api/tasks"):
		return 24 * time.Hour
	case looksImmutable(rawURL):
		return 0
	default:
		if c.CacheTTL > 0 {
			return c.CacheTTL
		}
		return 15 * time.Minute
	}
}

// looksImmutable reports whether a URL is pinned to a full commit sha, in which
// case its content cannot change.
func looksImmutable(rawURL string) bool {
	for _, seg := range strings.Split(rawURL, "/") {
		if len(seg) == 40 && isHex(seg) {
			return true
		}
	}
	return false
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}
