// Package hf is the library behind the hf command line:
// the HTTP client, request shaping, and typed data models for Hugging Face.
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
package hf

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultUserAgent identifies the client to Hugging Face. A real, honest
// User-Agent is both polite and the thing most likely to keep you unblocked.
const DefaultUserAgent = "hf/dev (+https://github.com/tamnd/hf-cli)"

// Host is the site this client talks to, and the host the URI driver in
// domain.go claims.
const Host = "huggingface.co"

// baseURL is the root every request is built from.
const baseURL = "https://huggingface.co"

// Client talks to Hugging Face over HTTP.
type Client struct {
	HTTP      *http.Client
	UserAgent string
	// Rate is the minimum gap between requests. Zero means no pacing.
	Rate    time.Duration
	Retries int

	last time.Time
}

// NewClient returns a Client with sensible defaults: a 30s timeout, a 200ms
// minimum gap between requests, and five retries on transient errors.
func NewClient() *Client {
	return &Client{
		HTTP:      &http.Client{Timeout: 30 * time.Second},
		UserAgent: DefaultUserAgent,
		Rate:      200 * time.Millisecond,
		Retries:   5,
	}
}

// Paper holds the fields callers care about for a Hugging Face daily paper.
type Paper struct {
	ID      string   `json:"id"      kit:"id" table:"id"`
	Title   string   `json:"title"            table:"title"`
	Upvotes int      `json:"upvotes"          table:"upvotes"`
	Authors []string `json:"authors"          table:"authors"`
	URL     string   `json:"url"              table:"url,url"`
}

// wire types for the API responses

type wireAuthor struct {
	Name string `json:"name"`
}

type wirePaper struct {
	ID      string       `json:"id"`
	Title   string       `json:"title"`
	Upvotes int          `json:"upvotes"`
	Authors []wireAuthor `json:"authors"`
}

type wireEntry struct {
	ID    string    `json:"id"`
	Paper wirePaper `json:"paper"`
}

// Top returns the top daily papers. date should be "YYYY-MM-DD"; if empty it
// defaults to yesterday UTC. It tries the daily_papers endpoint first and falls
// back to the papers endpoint if that returns no results.
func (c *Client) Top(ctx context.Context, date string, limit int) ([]*Paper, error) {
	if limit <= 0 {
		limit = 20
	}
	if date == "" {
		date = time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	}

	url := fmt.Sprintf("%s/api/daily_papers?date=%s&limit=%d", baseURL, date, limit)
	papers, err := c.fetchPapers(ctx, url)
	if err != nil || len(papers) == 0 {
		// fallback
		fallback := fmt.Sprintf("%s/api/papers?limit=%d", baseURL, limit)
		papers, err = c.fetchPapers(ctx, fallback)
		if err != nil {
			return nil, err
		}
	}
	return papers, nil
}

// fetchPapers fetches a papers list endpoint and decodes the response.
func (c *Client) fetchPapers(ctx context.Context, url string) ([]*Paper, error) {
	body, err := c.Get(ctx, url)
	if err != nil {
		return nil, err
	}

	var entries []wireEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decode papers: %w", err)
	}

	out := make([]*Paper, 0, len(entries))
	for _, e := range entries {
		p := toPaper(e)
		out = append(out, p)
	}
	return out, nil
}

// toPaper converts a wire entry to the public Paper type.
func toPaper(e wireEntry) *Paper {
	id := e.Paper.ID
	if id == "" {
		id = e.ID
	}

	// build author list (first 3)
	var authors []string
	for i, a := range e.Paper.Authors {
		if i >= 3 {
			break
		}
		if a.Name != "" {
			authors = append(authors, a.Name)
		}
	}

	return &Paper{
		ID:      id,
		Title:   e.Paper.Title,
		Upvotes: e.Paper.Upvotes,
		Authors: authors,
		URL:     baseURL + "/papers/" + id,
	}
}

// Get fetches url and returns the response body. It paces and retries according
// to the client's settings.
func (c *Client) Get(ctx context.Context, url string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, url)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", url, lastErr)
}

func (c *Client) do(ctx context.Context, url string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.Rate <= 0 {
		return
	}
	if wait := c.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// AuthorsString returns the first 3 authors joined by ", ".
func (p *Paper) AuthorsString() string {
	return strings.Join(p.Authors, ", ")
}
