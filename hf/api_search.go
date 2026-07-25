package hf

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// api_search.go covers the cross-entity surfaces: quicksearch, trending, the
// taxonomy, the task pages, and the sitemaps.

// Quicksearch is the only endpoint that answers for every entity kind at once.
// It returns the hits in the hub's own order, and the per-type counts alongside
// them so a caller can report what it did not show.
func (c *Client) Quicksearch(ctx context.Context, q string, kinds []string, limit int, emit func(*Hit) error) (*Counts, error) {
	if strings.TrimSpace(q) == "" {
		return nil, errs.Usage("search needs a query")
	}
	u := query(c.api("quicksearch"), "q", q, "type", strings.Join(kinds, ","))
	if limit > 0 {
		u = query(u, "limit", pageLimit(limit, 1, 100))
	}
	var res quicksearchResult
	resp, err := c.GetJSON(ctx, u, &res)
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		want[strings.TrimSuffix(k, "s")] = true
	}
	n := 0
	for _, g := range res.groups() {
		if len(want) > 0 && !want[g.Kind] {
			continue
		}
		for _, raw := range g.Rows {
			var h Hit
			if err := jsonUnmarshal(raw, &h); err != nil {
				return nil, err
			}
			h.normalize(g.Kind, resp.URL)
			if err := emit(&h); err != nil {
				return nil, err
			}
			n++
			if limit > 0 && n >= limit {
				goto done
			}
		}
	}
done:
	counts := res.Counts
	if counts == nil {
		counts = &Counts{}
	}
	counts.Query = q
	counts.Total = counts.Sum()
	counts.setMeta("", "search/"+q, resp.URL)
	return counts, nil
}

// Trending returns what the hub is showing on its front page, which is a
// different ranking from any sort the list endpoints offer.
func (c *Client) Trending(ctx context.Context, kind string, limit int, emit func(*Hit) error) error {
	u := query(c.api("trending"), "type", apiPath[kind], "limit", pageLimit(limit, 1, 100))
	var body struct {
		RecentlyTrending []struct {
			RepoType string          `json:"repoType"`
			RepoData json.RawMessage `json:"repoData"`
			Type     string          `json:"type"`
			Score    float64         `json:"trendingScore"`
		} `json:"recentlyTrending"`
	}
	resp, err := c.GetJSON(ctx, u, &body)
	if err != nil {
		return err
	}
	for i, row := range body.RecentlyTrending {
		if limit > 0 && i >= limit {
			return nil
		}
		k := row.RepoType
		if k == "" {
			k = row.Type
		}
		var h Hit
		if len(row.RepoData) > 0 {
			if err := jsonUnmarshal(row.RepoData, &h); err != nil {
				return err
			}
		}
		if h.TrendingWeight == 0 {
			h.TrendingWeight = int(row.Score)
		}
		h.normalize(k, resp.URL)
		if err := emit(&h); err != nil {
			return err
		}
	}
	return nil
}

// Taxonomy fetches the controlled vocabulary and caches it for the process.
// This is the authority that turns an opaque tag string into a typed node, so
// tag parsing is much weaker without it.
func (c *Client) Taxonomy(ctx context.Context) (*Taxonomy, error) {
	c.tagsMu.Lock()
	defer c.tagsMu.Unlock()
	if c.tags != nil {
		return c.tags, nil
	}
	tax := &Taxonomy{}
	var first *Response
	// The spaces document answers 401 anonymously, so only these two are read.
	for _, route := range []string{"models-tags-by-type", "datasets-tags-by-type"} {
		u := c.api(route)
		resp, err := c.Get(ctx, u)
		if err != nil {
			// One document missing degrades the vocabulary but does not break
			// it, and a partial taxonomy beats no taxonomy.
			if IsNeedAuth(err) || IsNotFound(err) {
				continue
			}
			return nil, err
		}
		var groups map[string][]Tag
		if err := jsonUnmarshal(resp.Body, &groups); err != nil {
			return nil, errs.Wrap(errs.KindNetwork, err, "decode %s", shortURL(u))
		}
		tax.Merge(groups)
		if first == nil {
			first = resp
		}
	}
	if first == nil {
		return nil, errs.Network("no taxonomy document could be read")
	}
	tax.setMeta("", "taxonomy", first.URL)
	c.tags = tax
	return tax, nil
}

// Tasks fetches the hub's curated task pages. Each one names the models,
// datasets, and spaces its editors chose, with prose reasons, which is an
// editorial signal and a different thing from a download count.
func (c *Client) Tasks(ctx context.Context) ([]Task, error) {
	u := c.api("tasks")
	resp, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var byID map[string]json.RawMessage
	if err := jsonUnmarshal(resp.Body, &byID); err != nil {
		return nil, errs.Wrap(errs.KindNetwork, err, "decode %s", shortURL(u))
	}
	out := make([]Task, 0, len(byID))
	for _, id := range sortedKeys(byID) {
		var t Task
		if err := jsonUnmarshal(byID[id], &t); err != nil {
			return nil, err
		}
		if t.ID == "" {
			t.ID = id
		}
		t.normalize(resp.URL)
		out = append(out, t)
	}
	return out, nil
}

// Task fetches one task by id.
func (c *Client) Task(ctx context.Context, id string) (*Task, error) {
	all, err := c.Tasks(ctx)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].ID == id {
			return &all[i], nil
		}
	}
	return nil, errs.NotFound("no task %q", id)
}

// sitemapIndex and sitemapURLSet are the two shapes a sitemap file can have.
type sitemapIndex struct {
	Sitemaps []struct {
		Loc     string `xml:"loc"`
		LastMod string `xml:"lastmod"`
	} `xml:"sitemap"`
}

type sitemapURLSet struct {
	URLs []struct {
		Loc        string `xml:"loc"`
		LastMod    string `xml:"lastmod"`
		ChangeFreq string `xml:"changefreq"`
		Priority   string `xml:"priority"`
	} `xml:"url"`
}

// Sitemap walks the sitemaps, following the index into each child set. This is
// the only enumeration channel that does not depend on an API cursor staying
// valid across a long run, which makes it the right seed for a full crawl.
func (c *Client) Sitemap(ctx context.Context, which string, limit int, emit func(*SitemapEntry) error) error {
	start := c.Base + "/sitemap.xml"
	if which != "" {
		start = c.Base + "/sitemap-" + which + ".xml"
	}
	n := 0
	queue := []string{start}
	seen := map[string]bool{start: true}
	for len(queue) > 0 {
		u := queue[0]
		queue = queue[1:]
		resp, err := c.Get(ctx, u)
		if err != nil {
			if IsNotFound(err) {
				continue
			}
			return err
		}
		var idx sitemapIndex
		if xml.Unmarshal(resp.Body, &idx) == nil && len(idx.Sitemaps) > 0 {
			for _, s := range idx.Sitemaps {
				if s.Loc != "" && !seen[s.Loc] {
					seen[s.Loc] = true
					queue = append(queue, s.Loc)
				}
			}
			continue
		}
		var set sitemapURLSet
		if err := xml.Unmarshal(resp.Body, &set); err != nil {
			return errs.Wrap(errs.KindNetwork, err, "decode %s", shortURL(u))
		}
		for _, row := range set.URLs {
			e := &SitemapEntry{
				Loc:        row.Loc,
				LastMod:    row.LastMod,
				ChangeFreq: row.ChangeFreq,
				Priority:   row.Priority,
			}
			e.normalize(resp.URL)
			if err := emit(e); err != nil {
				return err
			}
			n++
			if limit > 0 && n >= limit {
				return nil
			}
		}
	}
	return nil
}
