// Package hftest records real exchanges with huggingface.co and replays them.
// The tests in this repo run against the hub's actual answers rather than
// against payloads somebody wrote by hand, because the whole point of the tool
// is to survive what the hub really returns, and a hand-written fixture only
// ever contains what its author already knew about.
//
// A fixture is two files. The head holds the URL, the status, and the headers
// that matter. The body sits beside it in its own file so a recorded JSON
// document or HTML page stays readable in an editor and legible in a diff.
//
// Both halves are http.RoundTripper, so the client under test is the real one
// with its real URL building. Nothing is rewritten to point at a test server,
// which matters here because the page plane addresses huggingface.co directly
// rather than through the client's base URL.
package hftest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Head is everything about an exchange except the body.
type Head struct {
	URL      string      `json:"url"`
	FinalURL string      `json:"finalUrl,omitempty"`
	Status   int         `json:"status"`
	Header   http.Header `json:"header,omitempty"`
	Body     string      `json:"body"`
}

// keepHeaders are the response headers a replay has to reproduce for the client
// to behave the way it did live. Location is the one that matters most: file
// reads go through a redirect to the CDN, and a replay that dropped it would
// hand the caller the redirect instead of the file. Everything else is dropped,
// which keeps cookies and per-request tracing out of the repository.
var keepHeaders = []string{
	"Content-Type",
	"Link",
	"Location",
	"Retry-After",
	"X-Repo-Commit",
	"X-Linked-Size",
	"X-Linked-Etag",
	"ETag",
}

// Slug turns a URL into a filename a person can still read. The hash is there
// because URLs that differ only past the truncation point are common on the
// hub, and a fixture set with a silent collision in it is worse than none.
func Slug(rawURL string) string {
	sum := sha256.Sum256([]byte(rawURL))
	trimmed := rawURL
	for _, prefix := range []string{"https://huggingface.co/", "https://datasets-server.huggingface.co/"} {
		trimmed = strings.TrimPrefix(trimmed, prefix)
	}
	var b strings.Builder
	for _, r := range trimmed {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	name := b.String()
	if len(name) > 90 {
		name = name[:90]
	}
	return strings.Trim(name, "_") + "-" + hex.EncodeToString(sum[:4])
}

// Recorder is a transport that saves everything it fetches, so a recording run
// is an ordinary run that happens to leave fixtures behind.
type Recorder struct {
	Dir  string
	Next http.RoundTripper

	mu    sync.Mutex
	saved map[string]bool
}

// NewRecorder returns a recorder writing into dir, creating it if needed.
func NewRecorder(dir string, next http.RoundTripper) (*Recorder, error) {
	if next == nil {
		next = http.DefaultTransport
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Recorder{Dir: dir, Next: next, saved: map[string]bool{}}, nil
}

// RoundTrip fetches and saves. A failed save is reported rather than swallowed,
// because a recording run that silently wrote nothing is the failure that would
// not show up until the replay tests were already broken.
func (r *Recorder) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := r.Next.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	body, err := drain(resp)
	if err != nil {
		return nil, err
	}
	if err := r.save(req.URL.String(), resp, body); err != nil {
		return nil, err
	}
	return resp, nil
}

// Count reports how many distinct exchanges were saved.
func (r *Recorder) Count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.saved)
}

// drain reads a response and puts an equivalent body back, so whoever asked for
// it still gets to read it. The transport already undid any transfer encoding it
// negotiated, so what lands on disk is what the decoders will see.
func drain(resp *http.Response) ([]byte, error) {
	b, err := io.ReadAll(resp.Body)
	if cerr := resp.Body.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(b))
	return b, nil
}

func (r *Recorder) save(rawURL string, resp *http.Response, body []byte) error {
	slug := Slug(rawURL)
	r.mu.Lock()
	already := r.saved[slug]
	r.saved[slug] = true
	r.mu.Unlock()
	if already {
		return nil
	}

	name := slug + bodyExt(resp.Header.Get("Content-Type"))
	if err := os.WriteFile(filepath.Join(r.Dir, name), body, 0o644); err != nil {
		return err
	}
	head := Head{URL: rawURL, Status: resp.StatusCode, Body: name}
	for _, k := range keepHeaders {
		if v := resp.Header.Values(k); len(v) > 0 {
			if head.Header == nil {
				head.Header = http.Header{}
			}
			head.Header[k] = v
		}
	}
	if resp.Request != nil && resp.Request.URL != nil {
		if final := resp.Request.URL.String(); final != rawURL {
			head.FinalURL = final
		}
	}
	b, err := json.MarshalIndent(head, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(r.Dir, slug+".head.json"), append(b, '\n'), 0o644)
}

func bodyExt(contentType string) string {
	switch {
	case strings.Contains(contentType, "json"):
		return ".body.json"
	case strings.Contains(contentType, "html"):
		return ".body.html"
	case strings.Contains(contentType, "xml"):
		return ".body.xml"
	default:
		return ".body"
	}
}

// Exchange is one loaded fixture.
type Exchange struct {
	Head Head
	Body []byte
}

// Replayer answers from a recorded fixture set and never touches the network.
type Replayer struct {
	index map[string]Exchange

	mu     sync.Mutex
	misses map[string]bool
	hits   map[string]bool
}

// NewReplayer loads a fixture directory.
func NewReplayer(dir string) (*Replayer, error) {
	index, err := Load(dir)
	if err != nil {
		return nil, err
	}
	return &Replayer{index: index, misses: map[string]bool{}, hits: map[string]bool{}}, nil
}

// RoundTrip serves the recorded answer. A URL with no fixture is an error
// naming the URL, rather than a 404 that some decoder would go on to report as
// a missing repo: a test that quietly stopped covering an endpoint is exactly
// what this is here to catch.
func (r *Replayer) RoundTrip(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	ex, ok := r.index[Slug(url)]
	r.mu.Lock()
	if ok {
		r.hits[url] = true
	} else {
		r.misses[url] = true
	}
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no fixture for %s (re-record with: make fixtures)", url)
	}

	header := http.Header{}
	for k, v := range ex.Head.Header {
		header[k] = v
	}
	final := req
	if ex.Head.FinalURL != "" {
		if u, err := req.URL.Parse(ex.Head.FinalURL); err == nil {
			clone := req.Clone(req.Context())
			clone.URL = u
			final = clone
		}
	}
	return &http.Response{
		StatusCode:    ex.Head.Status,
		Status:        http.StatusText(ex.Head.Status),
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(ex.Body)),
		ContentLength: int64(len(ex.Body)),
		Request:       final,
	}, nil
}

// Misses lists the URLs asked for that no fixture covered, sorted.
func (r *Replayer) Misses() []string { return r.sortedKeys(r.misses) }

// Unused lists the fixtures nothing asked for, sorted. It is how a fixture set
// stays honest after a command stops making a request it used to make.
func (r *Replayer) Unused() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []string
	for _, ex := range r.index {
		if !r.hits[ex.Head.URL] {
			out = append(out, ex.Head.URL)
		}
	}
	sort.Strings(out)
	return out
}

func (r *Replayer) sortedKeys(m map[string]bool) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Load reads a whole fixture directory, keyed by slug.
func Load(dir string) (map[string]Exchange, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := map[string]Exchange{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".head.json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var head Head
		if err := json.Unmarshal(b, &head); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		body, err := os.ReadFile(filepath.Join(dir, head.Body))
		if err != nil {
			return nil, err
		}
		out[strings.TrimSuffix(e.Name(), ".head.json")] = Exchange{Head: head, Body: body}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s holds no fixtures", dir)
	}
	return out, nil
}
