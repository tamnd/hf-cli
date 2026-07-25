package hf

import (
	"encoding/json"
	"strings"
	"time"
)

// social.go covers posts, the blog, comments, and reactions. The interesting
// property of hub posts is that their body arrives as a token array rather than
// markdown, so entity mentions are structural rather than something to parse
// back out of prose.

// Post is a social post.
type Post struct {
	Meta

	Slug        string       `json:"slug"`
	ObjectID    string       `json:"_id,omitempty"`
	Author      *UserRef     `json:"author,omitempty"`
	Content     []PostToken  `json:"content,omitempty"`
	Body        string       `json:"body,omitempty"`
	PublishedAt time.Time    `json:"publishedAt,omitzero"`
	UpdatedAt   time.Time    `json:"updatedAt,omitzero"`
	NumComments int          `json:"numComments,omitempty"`
	Reactions   []Reaction   `json:"reactions,omitempty"`
	Comments    []Comment    `json:"comments,omitempty"`
	IsPinned    bool         `json:"isPinned,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
	Identifier  string       `json:"identifier,omitempty"`

	// Commentators is who replied, without fetching the replies. Mentions is who
	// the post named, already resolved to accounts, which is the one social edge
	// the hub draws out of free text for you.
	Commentators []UserRef `json:"commentators,omitempty"`
	Mentions     []UserRef `json:"mentions,omitempty"`

	// TotalUniqueImpressions is the only reach number the hub publishes, and it
	// is absent on posts too old or too new to have one.
	TotalUniqueImpressions int `json:"totalUniqueImpressions,omitempty"`

	// IdentifiedLanguage is the hub's own guess, probability included, so a
	// low-confidence guess can be told apart from a confident one.
	IdentifiedLanguage *LanguageGuess `json:"identifiedLanguage,omitempty"`
}

// LanguageGuess is a language code and how sure the classifier was.
type LanguageGuess struct {
	Language    string  `json:"language"`
	Probability float64 `json:"probability,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra. The
// source markdown arrives as rawContent and lands in Body, because a post has
// one body and carrying it twice would double the size of a feed dump.
func (p *Post) UnmarshalJSON(b []byte) error {
	type raw Post
	if err := decodeExtra(b, (*raw)(p), &p.Extra, "rawContent"); err != nil {
		return err
	}
	if p.Body == "" {
		var alt struct {
			RawContent string `json:"rawContent"`
		}
		if json.Unmarshal(b, &alt) == nil {
			p.Body = alt.RawContent
		}
	}
	return nil
}

func (p *Post) normalize(sourceURL string) {
	if p.Author != nil {
		p.Author.normalize()
	}
	for i := range p.Commentators {
		p.Commentators[i].normalize()
	}
	for i := range p.Mentions {
		p.Mentions[i].normalize()
	}
	if p.Body == "" {
		p.Body = renderTokens(p.Content)
	}
	for i := range p.Content {
		p.Content[i].normalize()
	}
	id := p.Slug
	if p.Author != nil && p.Author.Name != "" && !strings.Contains(id, "/") {
		id = p.Author.Name + "/" + id
	}
	p.setMeta(KindPost, id, sourceURL)
}

// PostToken is one span of a post body.
type PostToken struct {
	Type     string       `json:"type"`
	Value    string       `json:"value,omitempty"`
	Raw      string       `json:"raw,omitempty"`
	User     string       `json:"user,omitempty"`
	Resource *ResourceRef `json:"resource,omitempty"`
	URL      string       `json:"url,omitempty"`
	Lang     string       `json:"lang,omitempty"`
	Label    string       `json:"label,omitempty"`
}

func (t *PostToken) normalize() {
	if t.Resource != nil {
		t.Resource.normalize()
	}
}

// renderTokens reassembles a token array into readable text. It is the reverse
// of what the site does, and it exists so a post has a body a person can grep.
func renderTokens(toks []PostToken) string {
	var b strings.Builder
	for _, t := range toks {
		switch t.Type {
		case "new_line":
			b.WriteString("\n")
		case "mention":
			b.WriteString("@" + t.User)
		case "resource-link":
			if t.Resource != nil {
				b.WriteString(t.Resource.ID)
			} else {
				b.WriteString(t.Raw)
			}
		case "code":
			b.WriteString("`" + t.Value + "`")
		case "link", "image":
			if t.Value != "" {
				b.WriteString(t.Value)
			} else {
				b.WriteString(t.URL)
			}
		default:
			if t.Value != "" {
				b.WriteString(t.Value)
			} else {
				b.WriteString(t.Raw)
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// ResourceRef is a hub entity named inside a post or a comment. It is a real
// edge that the author typed on purpose, which makes it worth more than a
// heuristic link extraction.
type ResourceRef struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	URI  string `json:"uri,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (r *ResourceRef) normalize() {
	kind := r.Type
	switch r.Type {
	case "":
		return
	case "org", "user":
		kind = r.Type
	}
	r.URI = URI(kind, r.ID)
	if u, err := Locate(kind, r.ID); err == nil {
		r.URL = u
	}
}

// Reaction is one emoji and who used it.
type Reaction struct {
	Reaction string   `json:"reaction"`
	Users    []string `json:"users,omitempty"`
	Count    int      `json:"count"`
}

// Attachment is a file carried by a post.
type Attachment struct {
	Type string `json:"type,omitempty"`
	URL  string `json:"url,omitempty"`
	Name string `json:"name,omitempty"`
	Size int64  `json:"size,omitempty"`
}

// Comment is one reply, on a discussion, a paper, a post, or a blog article.
type Comment struct {
	ID        string        `json:"id,omitempty"`
	Author    *UserRef      `json:"author,omitempty"`
	CreatedAt time.Time     `json:"createdAt,omitzero"`
	EditedAt  time.Time     `json:"editedAt,omitzero"`
	Raw       string        `json:"raw,omitempty"`
	HTML      string        `json:"html,omitempty"`
	Hidden    bool          `json:"hidden,omitempty"`
	Edited    bool          `json:"edited,omitempty"`
	Reactions []Reaction    `json:"reactions,omitempty"`
	Mentions  []ResourceRef `json:"mentions,omitempty"`
}

// BlogPost is an article on the hub blog. The index comes from the RSS feed,
// which is the only listing endpoint the blog has.
type BlogPost struct {
	Meta

	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	PublishedAt time.Time `json:"publishedAt,omitzero"`
	GUID        string    `json:"guid,omitempty"`
	Lang        string    `json:"lang,omitempty"`
	Thumbnail   string    `json:"thumbnail,omitempty"`
	Tags        []string  `json:"tags,omitempty"`

	Authors      []UserRef `json:"authors,omitempty"`
	Translators  []UserRef `json:"translators,omitempty"`
	Proofreaders []UserRef `json:"proofreaders,omitempty"`

	Upvotes  int       `json:"upvotes,omitempty"`
	Upvoters []UserRef `json:"upvoters,omitempty"`
	Comments []Comment `json:"comments,omitempty"`

	Blocks []Block `json:"blocks,omitempty"`
	Body   string  `json:"body,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (b *BlogPost) UnmarshalJSON(data []byte) error {
	type raw BlogPost
	return decodeExtra(data, (*raw)(b), &b.Extra)
}

// Block is one structural element of rendered prose. A blog body, an org card,
// and a rendered README all decompose into these, which is what lets the tool
// emit prose as data rather than as a wall of HTML.
type Block struct {
	Type  string     `json:"type"`
	Text  string     `json:"text,omitempty"`
	Level int        `json:"level,omitempty"`
	Lang  string     `json:"lang,omitempty"`
	Href  string     `json:"href,omitempty"`
	Src   string     `json:"src,omitempty"`
	Alt   string     `json:"alt,omitempty"`
	Items []string   `json:"items,omitempty"`
	Rows  [][]string `json:"rows,omitempty"`
}

// Like is a timestamped edge from a user to a repo.
type Like struct {
	Meta

	User      string    `json:"user"`
	CreatedAt time.Time `json:"createdAt,omitzero"`
	Repo      *RepoRef  `json:"repo,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (l *Like) UnmarshalJSON(b []byte) error {
	type raw Like
	return decodeExtra(b, (*raw)(l), &l.Extra)
}

// RepoRef names a repo from inside another record.
type RepoRef struct {
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
	URI  string `json:"uri,omitempty"`
	URL  string `json:"url,omitempty"`
}

func (r *RepoRef) normalize() {
	kind := r.Type
	if kind == "" {
		kind = KindModel
	}
	r.URI = URI(kind, r.Name)
	if u, err := Locate(kind, r.Name); err == nil {
		r.URL = u
	}
}

var _ = json.Marshal
