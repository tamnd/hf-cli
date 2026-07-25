package hf

import (
	"bytes"
	"context"
	"encoding/json"
	"html"
	"regexp"
	"strings"
)

// page.go is the HTML plane. huggingface.co is server-rendered Svelte with
// client-side hydration, so every interactive component is emitted with a
// data-props attribute holding the exact JSON object it was rendered from. That
// makes a page already a 1:1 JSON structure: reconstructing it is not something
// to approximate, it is the site's own architecture.

// Page is the whole of one rendered page as data.
type Page struct {
	Meta

	Title     string `json:"title,omitempty" table:"title,truncate"`
	Canonical string `json:"canonical,omitempty" table:"-"`

	// Components is the hydration payloads keyed by component name. The value
	// is a slice because a name can repeat: a blog post carries two
	// BlogThumbnail blocks, a repo page several CopyButton blocks.
	Components map[string][]json.RawMessage `json:"components,omitempty" table:"components"`

	// LD is every application/ld+json block, and Head is the meta tag set.
	LD   []json.RawMessage `json:"ld,omitempty" table:"-"`
	Head map[string]string `json:"meta,omitempty" table:"-"`

	// HTML is kept only when the caller asked for the body, because the DOM
	// path is the one thing that needs it.
	HTML []byte `json:"-" table:"-"`
}

// Component returns the first payload for a name, which is what a record
// builder wants. Callers that need all of them read Components directly.
func (p *Page) Component(name string) json.RawMessage {
	if p == nil {
		return nil
	}
	if v := p.Components[name]; len(v) > 0 {
		return v[0]
	}
	return nil
}

// Into decodes one component's payload. A missing component is not an error:
// every page-derived field is optional, and a record built without one is
// exactly what the API gave.
func (p *Page) Into(name string, v any) bool {
	raw := p.Component(name)
	if len(raw) == 0 {
		return false
	}
	return jsonUnmarshal(raw, v) == nil
}

// Has reports whether any of the named components is present, which is how a
// builder decides whether the page gave it anything at all.
func (p *Page) Has(names ...string) bool {
	for _, n := range names {
		if len(p.Components[n]) > 0 {
			return true
		}
	}
	return false
}

// maxPayload caps one data-props value. The org and dataset payloads run to
// around 250 KB, so this is far above anything real, and it means a
// pathological page cannot exhaust memory.
const maxPayload = 8 << 20

// denyKeys are session artifacts and secrets. They are stripped at extraction
// rather than at output, so `hf page` is safe to paste into an issue.
var denyKeys = map[string]bool{
	"csrf":                 true,
	"jwt":                  true,
	"sessionUuid":          true,
	"postLoginRedirectUrl": true,
	"apiUrlPrefix":         true,
}

var denyPattern = regexp.MustCompile(`(?i)token|secret|password`)

// Page fetches and extracts one page.
func (c *Client) Page(ctx context.Context, rawURL string) (*Page, error) {
	resp, err := c.Get(ctx, rawURL)
	if err != nil {
		return nil, err
	}
	p := ParsePage(resp.Body)
	p.URL = rawURL
	if resp.FinalURL != "" {
		p.URL = resp.FinalURL
	}
	p.addSource(rawURL)
	if kind, id, cerr := Classify(p.URL); cerr == nil {
		p.Kind = kind
		p.Meta.URI = URI(kind, id)
	}
	return p, nil
}

// PageOf fetches the page for an entity.
func (c *Client) PageOf(ctx context.Context, kind, id string) (*Page, error) {
	u, err := Locate(kind, id)
	if err != nil {
		return nil, err
	}
	return c.Page(ctx, u)
}

// ParsePage extracts everything readable out of a rendered page.
//
// This does not need a DOM. data-target and data-props are attributes on a
// single tag, always in that order, always double-quoted, and data-props never
// contains a raw quote because the value is entity-escaped. A byte scanner is
// enough, and it is far faster than parsing a 250 KB document into a tree.
func ParsePage(body []byte) *Page {
	p := &Page{
		Components: map[string][]json.RawMessage{},
		Head:       map[string]string{},
		HTML:       body,
	}
	scanHydrators(body, p)
	scanLD(body, p)
	scanHead(body, p)
	return p
}

var (
	targetAttr = []byte(`data-target="`)
	propsAttr  = []byte(`data-props="`)
)

// maxGap is how far a data-props may sit from its data-target. Bounding it
// means a payload belonging to a later element is never paired with an earlier
// target.
const maxGap = 512

func scanHydrators(body []byte, p *Page) {
	pos := 0
	for {
		i := bytes.Index(body[pos:], targetAttr)
		if i < 0 {
			return
		}
		i += pos + len(targetAttr)
		end := bytes.IndexByte(body[i:], '"')
		if end < 0 {
			return
		}
		name := string(body[i : i+end])
		rest := i + end

		limit := rest + maxGap
		if limit > len(body) {
			limit = len(body)
		}
		j := bytes.Index(body[rest:limit], propsAttr)
		if j < 0 {
			pos = rest
			continue
		}
		j += rest + len(propsAttr)
		pend := bytes.IndexByte(body[j:], '"')
		if pend < 0 {
			return
		}
		pos = j + pend

		if pend > maxPayload {
			continue
		}
		raw := html.UnescapeString(string(body[j : j+pend]))
		var v any
		if json.Unmarshal([]byte(raw), &v) != nil {
			continue
		}
		v = scrub(v)
		clean, err := json.Marshal(v)
		if err != nil {
			continue
		}
		p.Components[name] = append(p.Components[name], clean)
	}
}

// scrub removes the deny-listed keys anywhere in a payload. It walks the whole
// tree because a session token nested three levels down is still a session
// token.
func scrub(v any) any {
	switch t := v.(type) {
	case map[string]any:
		for k := range t {
			if denyKeys[k] || denyPattern.MatchString(k) {
				delete(t, k)
				continue
			}
			t[k] = scrub(t[k])
		}
		return t
	case []any:
		for i := range t {
			t[i] = scrub(t[i])
		}
		return t
	default:
		return v
	}
}

var ldOpen = regexp.MustCompile(`(?i)<script[^>]+type=["']application/ld\+json["'][^>]*>`)

func scanLD(body []byte, p *Page) {
	for _, loc := range ldOpen.FindAllIndex(body, -1) {
		rest := body[loc[1]:]
		end := bytes.Index(rest, []byte("</script>"))
		if end < 0 {
			continue
		}
		text := html.UnescapeString(string(rest[:end]))
		var v any
		if json.Unmarshal([]byte(text), &v) != nil {
			continue
		}
		clean, err := json.Marshal(v)
		if err != nil {
			continue
		}
		p.LD = append(p.LD, clean)
	}
}

var (
	titleRe     = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaRe      = regexp.MustCompile(`(?i)<meta\s+[^>]*>`)
	canonicalRe = regexp.MustCompile(`(?i)<link\s+[^>]*rel=["']canonical["'][^>]*>`)
	attrRe      = regexp.MustCompile(`(?i)([a-z:_-]+)\s*=\s*"([^"]*)"`)
)

func scanHead(body []byte, p *Page) {
	if m := titleRe.FindSubmatch(body); m != nil {
		p.Title = strings.TrimSpace(html.UnescapeString(string(m[1])))
	}
	for _, tag := range metaRe.FindAll(body, -1) {
		attrs := attrsOf(tag)
		key := attrs["property"]
		if key == "" {
			key = attrs["name"]
		}
		if key == "" || attrs["content"] == "" {
			continue
		}
		p.Head[key] = attrs["content"]
	}
	if tag := canonicalRe.Find(body); tag != nil {
		p.Canonical = attrsOf(tag)["href"]
	}
}

func attrsOf(tag []byte) map[string]string {
	out := map[string]string{}
	for _, m := range attrRe.FindAllSubmatch(tag, -1) {
		out[strings.ToLower(string(m[1]))] = html.UnescapeString(string(m[2]))
	}
	return out
}

// Thumbnail is the generated social card, which is the only image a repo has.
func (p *Page) Thumbnail() string {
	if v := p.Head["og:image"]; v != "" {
		return v
	}
	return p.Head["twitter:image"]
}

// Description is the page's own human summary. On a space it is often better
// prose than the card.
func (p *Page) Description() string {
	if v := p.Head["og:description"]; v != "" {
		return v
	}
	return p.Head["description"]
}
