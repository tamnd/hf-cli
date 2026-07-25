package hf

import (
	"encoding/json"
	"time"
)

// search.go covers the cross-entity search surface: quicksearch, which is the
// only endpoint that answers for every kind at once, and the sitemaps, which
// are the only complete enumeration the hub publishes.

// Hit is the uniform shape quicksearch results collapse to, so one command can
// search every entity kind at once.
type Hit struct {
	Meta

	Type           string `json:"type" table:"type"`
	ID             string `json:"id" table:"id"`
	ObjectID       string `json:"_id,omitempty" table:"-"`
	Label          string `json:"label,omitempty" table:"-"`
	Private        bool   `json:"private,omitempty" table:"-"`
	Gated          Gated  `json:"gated,omitempty" table:"-"`
	TrendingWeight int    `json:"trendingWeight,omitempty" table:"-"`
	Likes          int    `json:"likes,omitempty" table:"likes"`
	Downloads      int    `json:"downloads,omitempty" table:"downloads"`
	AvatarURL      string `json:"avatarUrl,omitempty" table:"-"`
	Fullname       string `json:"fullname,omitempty" table:"-"`

	// The trending feed hands back a repo summary rather than a search label, so
	// a hit carries the summary fields too. They stay empty for a quicksearch
	// result, which only ever knows the id.
	Author             string             `json:"author,omitempty" table:"-"`
	AuthorData         *UserRef           `json:"authorData,omitempty" table:"-"`
	LastModified       time.Time          `json:"lastModified,omitzero" table:"-"`
	PipelineTag        string             `json:"pipeline_tag,omitempty" table:"-"`
	NumParameters      int64              `json:"numParameters,omitempty" table:"-"`
	InferenceProviders InferenceProviders `json:"availableInferenceProviders,omitempty" table:"-"`
	IsLikedByUser      bool               `json:"isLikedByUser,omitempty" table:"-"`
	WidgetOutputURLs   []string           `json:"widgetOutputUrls,omitempty" table:"-"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra. A
// trending row names the kind repoType, so read that as the type rather than
// carrying two fields that say the same thing.
func (h *Hit) UnmarshalJSON(b []byte) error {
	type raw Hit
	if err := decodeExtra(b, (*raw)(h), &h.Extra, "repoType"); err != nil {
		return err
	}
	if h.Type == "" {
		var alt struct {
			RepoType string `json:"repoType"`
		}
		if json.Unmarshal(b, &alt) == nil {
			h.Type = alt.RepoType
		}
	}
	return nil
}

// normalize gives each hit an address. Quicksearch groups its results by kind
// and does not repeat the kind inside each entry, so the caller passes it in.
func (h *Hit) normalize(kind, sourceURL string) {
	if h.Type == "" {
		h.Type = kind
	}
	id := h.ID
	if id == "" {
		id = h.Label
	}
	if h.Label == "" {
		h.Label = id
	}
	if h.AuthorData != nil {
		h.AuthorData.normalize()
	}
	h.setMeta(h.Type, id, sourceURL)
}

// Counts is the per-type total quicksearch returns alongside the hits, which is
// how a search command reports what it did not show.
type Counts struct {
	Meta

	Query       string `json:"query" table:"query"`
	Models      int    `json:"models" table:"models"`
	Datasets    int    `json:"datasets" table:"datasets"`
	Spaces      int    `json:"spaces" table:"spaces"`
	Papers      int    `json:"papers" table:"papers"`
	Collections int    `json:"collections" table:"collections"`
	Kernels     int    `json:"kernels" table:"kernels"`
	Users       int    `json:"users" table:"users"`
	Orgs        int    `json:"orgs" table:"orgs"`
	Total       int    `json:"total" table:"total"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (c *Counts) UnmarshalJSON(b []byte) error {
	type raw Counts
	return decodeExtra(b, (*raw)(c), &c.Extra)
}

// Sum recomputes the total, because upstream does not send one.
func (c *Counts) Sum() int {
	return c.Models + c.Datasets + c.Spaces + c.Papers +
		c.Collections + c.Kernels + c.Users + c.Orgs
}

// quicksearchResult is the wire shape: one array per kind plus a counts object.
type quicksearchResult struct {
	Models      []json.RawMessage `json:"models"`
	Datasets    []json.RawMessage `json:"datasets"`
	Spaces      []json.RawMessage `json:"spaces"`
	Papers      []json.RawMessage `json:"papers"`
	Collections []json.RawMessage `json:"collections"`
	Kernels     []json.RawMessage `json:"kernels"`
	Users       []json.RawMessage `json:"users"`
	Orgs        []json.RawMessage `json:"orgs"`
	Counts      *Counts           `json:"counts,omitempty"`
}

// groups pairs each result array with the kind it belongs to, in the order the
// hub shows them.
func (q *quicksearchResult) groups() []struct {
	Kind string
	Rows []json.RawMessage
} {
	return []struct {
		Kind string
		Rows []json.RawMessage
	}{
		{KindModel, q.Models},
		{KindDataset, q.Datasets},
		{KindSpace, q.Spaces},
		{KindKernel, q.Kernels},
		{KindPaper, q.Papers},
		{KindCollection, q.Collections},
		{KindUser, q.Users},
		{KindOrg, q.Orgs},
	}
}

// SitemapEntry is one URL from a sitemap. The sitemaps are the only place the
// hub publishes a complete list of anything, which makes them the backstop when
// a listing endpoint caps out.
type SitemapEntry struct {
	Meta

	Loc        string `json:"loc" table:"loc"`
	LastMod    string `json:"lastmod,omitempty" table:"lastmod"`
	ChangeFreq string `json:"changefreq,omitempty" table:"-"`
	Priority   string `json:"priority,omitempty" table:"-"`
}

// normalize classifies the URL back into a kind and an id, so a sitemap sweep
// produces addressable records rather than a list of links.
func (s *SitemapEntry) normalize(sourceURL string) {
	kind, id, err := Classify(s.Loc)
	if err != nil {
		s.URL = s.Loc
		s.addSource(sourceURL)
		return
	}
	s.setMeta(kind, id, sourceURL)
}
