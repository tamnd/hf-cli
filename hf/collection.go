package hf

import (
	"strings"
	"time"
)

// Collection is a curated list. It is the one place on the hub where a human
// writes down why two things belong together, which makes it the highest signal
// per byte of anything the API returns.
type Collection struct {
	Meta

	Slug        string    `json:"slug"`
	Namespace   string    `json:"namespace,omitempty"`
	ShortSlug   string    `json:"shortSlug,omitempty"`
	ObjectID    string    `json:"_id,omitempty"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Gating      bool      `json:"gating,omitempty"`
	Theme       string    `json:"theme,omitempty"`
	Position    int       `json:"position,omitempty"`
	Private     bool      `json:"private,omitempty"`
	LastUpdated time.Time `json:"lastUpdated,omitzero"`
	Upvotes     int       `json:"upvotes,omitempty"`

	// IsUpvotedByUser is relative to the token making the request.
	IsUpvotedByUser bool `json:"isUpvotedByUser,omitempty"`

	// ShareURL is the hub's own social card link, which is not the page URL and
	// is the only short form of a collection address the site publishes.
	ShareURL string `json:"shareUrl,omitempty"`

	Owner *UserRef         `json:"owner,omitempty"`
	Items []CollectionItem `json:"items,omitempty"`

	// Page-derived.
	Upvoters []UserRef `json:"upvoters,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (c *Collection) UnmarshalJSON(b []byte) error {
	type raw Collection
	return decodeExtra(b, (*raw)(c), &c.Extra)
}

// normalize splits the slug into its three usable parts. All three are needed:
// the full slug addresses the API, the short slug plus namespace addresses the
// page, and the object id is the join key.
func (c *Collection) normalize(sourceURL string) {
	if ns, rest, ok := strings.Cut(c.Slug, "/"); ok {
		c.Namespace = ns
		if i := strings.LastIndexByte(rest, '-'); i > 0 {
			c.ShortSlug = rest[:i]
			if c.ObjectID == "" {
				c.ObjectID = rest[i+1:]
			}
		} else {
			c.ShortSlug = rest
		}
	}
	if c.Owner != nil {
		c.Owner.normalize()
	}
	for i := range c.Items {
		c.Items[i].normalize()
	}
	c.setMeta(KindCollection, c.Slug, sourceURL)
}

// CollectionItem is one member. It carries a denormalised copy of the member's
// own record, which is why it has fields such as numParameters that the
// member's own endpoint does not return.
type CollectionItem struct {
	ObjectID string `json:"_id,omitempty"`
	Position int    `json:"position"`
	Type     string `json:"type"`
	ID       string `json:"id"`
	URI      string `json:"uri,omitempty"`
	URL      string `json:"url,omitempty"`
	// Note is the curator's own comment on this item, and it is the most
	// interesting text on a collection.
	Note any `json:"note,omitempty"`

	Author                      string             `json:"author,omitempty"`
	AuthorData                  *UserRef           `json:"authorData,omitempty"`
	RepoType                    string             `json:"repoType,omitempty"`
	Downloads                   int                `json:"downloads,omitempty"`
	Likes                       int                `json:"likes,omitempty"`
	Gated                       Gated              `json:"gated,omitempty"`
	Private                     bool               `json:"private,omitempty"`
	PipelineTag                 string             `json:"pipeline_tag,omitempty"`
	LibraryName                 string             `json:"library_name,omitempty"`
	NumParameters               int64              `json:"numParameters,omitempty"`
	LastModified                time.Time          `json:"lastModified,omitzero"`
	AvailableInferenceProviders InferenceProviders `json:"availableInferenceProviders,omitempty"`
	WidgetOutputURLs            []string           `json:"widgetOutputUrls,omitempty"`
	IsLikedByUser               bool               `json:"isLikedByUser,omitempty"`

	// Paper items.
	Title       string    `json:"title,omitempty"`
	Upvotes     int       `json:"upvotes,omitempty"`
	Authors     []string  `json:"authors,omitempty"`
	PublishedAt time.Time `json:"publishedAt,omitzero"`

	// Space items.
	Runtime   *Runtime `json:"runtime,omitempty"`
	SDK       string   `json:"sdk,omitempty"`
	Emoji     string   `json:"emoji,omitempty"`
	ColorFrom string   `json:"colorFrom,omitempty"`
	ColorTo   string   `json:"colorTo,omitempty"`
}

// normalize gives the item its own address, so every member of a collection is
// independently followable.
func (i *CollectionItem) normalize() {
	if i.AuthorData != nil {
		i.AuthorData.normalize()
	}
	kind := i.Kind()
	if kind == "" || i.ID == "" {
		return
	}
	i.URI = URI(kind, i.ID)
	if u, err := Locate(kind, i.ID); err == nil {
		i.URL = u
	}
}

// Kind maps the item's own type name onto an hf kind.
func (i *CollectionItem) Kind() string {
	switch i.Type {
	case "model", "dataset", "space", "paper", "collection", "kernel":
		return i.Type
	default:
		return i.Type
	}
}

// NoteText renders the curator note, which upstream sends as either a plain
// string or an object with a text field.
func (i *CollectionItem) NoteText() string {
	switch n := i.Note.(type) {
	case string:
		return n
	case map[string]any:
		if s, ok := n["text"].(string); ok {
			return s
		}
	}
	return ""
}
