package hf

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"strings"
	"time"

	"github.com/tamnd/any-cli/kit/errs"
)

// api_ns.go fetches namespaces, collections, papers, posts, and the blog.

// User fetches one account. The overview route does not return the name,
// because it is the path parameter, so it is filled back in here.
func (c *Client) User(ctx context.Context, name string) (*User, error) {
	var u User
	resp, err := c.GetJSON(ctx, c.api("users", name, "overview"), &u)
	if err != nil {
		return nil, err
	}
	if u.Name == "" {
		u.Name = name
	}
	u.Type = KindUser
	for i := range u.Orgs {
		u.Orgs[i].Type = KindOrg
		u.Orgs[i].normalize()
	}
	u.setMeta(KindUser, u.Name, resp.URL)
	c.nsKind.Store(name, KindUser)
	return &u, nil
}

// Org fetches one organisation.
func (c *Client) Org(ctx context.Context, name string) (*Org, error) {
	var o Org
	resp, err := c.GetJSON(ctx, c.api("organizations", name, "overview"), &o)
	if err != nil {
		return nil, err
	}
	if o.Name == "" {
		o.Name = name
	}
	o.Type = KindOrg
	o.setMeta(KindOrg, o.Name, resp.URL)
	c.nsKind.Store(name, KindOrg)
	return &o, nil
}

// NamespaceKind answers whether a name is a user or an org. There is no
// endpoint that says, so it probes the org route first and falls back to the
// user route on a 404, then remembers the answer for the process.
func (c *Client) NamespaceKind(ctx context.Context, name string) (string, error) {
	if v, ok := c.nsKind.Load(name); ok {
		return v.(string), nil
	}
	if _, err := c.Get(ctx, c.api("organizations", name, "overview")); err == nil {
		c.nsKind.Store(name, KindOrg)
		return KindOrg, nil
	} else if !IsNotFound(err) {
		return "", err
	}
	if _, err := c.Get(ctx, c.api("users", name, "overview")); err != nil {
		return "", err
	}
	c.nsKind.Store(name, KindUser)
	return KindUser, nil
}

// Namespace fetches whichever kind the name turns out to be.
func (c *Client) Namespace(ctx context.Context, name string) (any, error) {
	kind, err := c.NamespaceKind(ctx, name)
	if err != nil {
		return nil, err
	}
	if kind == KindOrg {
		return c.Org(ctx, name)
	}
	return c.User(ctx, name)
}

// The social list endpoints reject a limit under 10, so the request-side limit
// is clamped up and the caller's smaller limit is applied locally. That is what
// makes `hf followers google -n 3` work.
const socialMinLimit = 10

// Followers walks the accounts following a namespace.
func (c *Client) Followers(ctx context.Context, name string, limit int, emit func(*UserRef) error) error {
	kind, err := c.NamespaceKind(ctx, name)
	if err != nil {
		return err
	}
	seg := "users"
	if kind == KindOrg {
		seg = "organizations"
	}
	u := query(c.api(seg, name, "followers"), "limit", pageLimit(limit, socialMinLimit, 1000))
	return walk(ctx, c, u, limit, decodeUserRef, emit)
}

// Following walks the accounts a user follows. Organisations do not follow.
func (c *Client) Following(ctx context.Context, name string, limit int, emit func(*UserRef) error) error {
	u := query(c.api("users", name, "following"), "limit", pageLimit(limit, socialMinLimit, 1000))
	return walk(ctx, c, u, limit, decodeUserRef, emit)
}

// Members walks an organisation's members. This is the reverse of the orgs
// array on a user overview.
func (c *Client) Members(ctx context.Context, name string, limit int, emit func(*UserRef) error) error {
	u := query(c.api("organizations", name, "members"), "limit", pageLimit(limit, socialMinLimit, 1000))
	return walk(ctx, c, u, limit, decodeUserRef, emit)
}

// Likes walks what a user liked, newest first. A like is a timestamped edge to
// a repo of any kind, and it is the cleanest interest signal on the site.
func (c *Client) Likes(ctx context.Context, name string, limit int, emit func(*Like) error) error {
	u := query(c.api("users", name, "likes"), "limit", pageLimit(limit, 1, 1000))
	return walk(ctx, c, u, limit, func(raw json.RawMessage, resp *Response) (*Like, error) {
		var l Like
		if err := jsonUnmarshal(raw, &l); err != nil {
			return nil, err
		}
		l.User = name
		if l.Repo != nil {
			l.Repo.normalize()
			// A like is an edge, so its identity is both ends. Keying it on the
			// repo alone would collapse every liker of a repo into one row.
			l.setMeta(KindUser, name+"#likes/"+l.Repo.Type+"/"+l.Repo.Name, resp.URL)
			l.URL = l.Repo.URL
		}
		return &l, nil
	}, emit)
}

// CollectionOptions filters a collection list. Item is the reverse edge: it
// finds every collection containing a given entity, expressed as models/{id},
// datasets/{id}, spaces/{id}, or papers/{id}.
type CollectionOptions struct {
	Owner string
	Item  string
	Sort  string
	Limit int
}

// Collections walks the collection list.
func (c *Client) Collections(ctx context.Context, o CollectionOptions, emit func(*Collection) error) error {
	u := query(c.api("collections"),
		"owner", o.Owner,
		"item", o.Item,
		"sort", o.Sort,
		"limit", pageLimit(o.Limit, 1, 1000),
	)
	return walk(ctx, c, u, o.Limit, func(raw json.RawMessage, resp *Response) (*Collection, error) {
		var col Collection
		if err := jsonUnmarshal(raw, &col); err != nil {
			return nil, err
		}
		col.normalize(resp.URL)
		return &col, nil
	}, emit)
}

// Collection fetches one collection by its full slug, which is
// {namespace}/{title-slug}-{24 hex chars}.
func (c *Client) Collection(ctx context.Context, slug string) (*Collection, error) {
	ns, rest, ok := strings.Cut(strings.Trim(slug, "/"), "/")
	if !ok {
		return nil, errs.Usage("a collection slug is namespace/title-hexid, got %q", slug)
	}
	var col Collection
	resp, err := c.GetJSON(ctx, c.api("collections", ns, rest), &col)
	if err != nil {
		return nil, err
	}
	if col.Slug == "" {
		col.Slug = slug
	}
	col.normalize(resp.URL)
	return &col, nil
}

// Paper fetches one paper by its arXiv id.
func (c *Client) Paper(ctx context.Context, id string) (*Paper, error) {
	id = trimVersion(id)
	var p Paper
	resp, err := c.GetJSON(ctx, c.api("papers", id), &p)
	if err != nil {
		return nil, err
	}
	if p.ID == "" {
		p.ID = id
	}
	p.normalize(resp.URL)
	return &p, nil
}

// SearchPapers runs the paper search, which is not paginated.
func (c *Client) SearchPapers(ctx context.Context, q string, limit int, emit func(*Paper) error) error {
	u := query(c.api("papers", "search"), "q", q)
	resp, err := c.Get(ctx, u)
	if err != nil {
		return err
	}
	var items []json.RawMessage
	if err := jsonUnmarshal(resp.Body, &items); err != nil {
		return errs.Wrap(errs.KindNetwork, err, "decode %s", shortURL(u))
	}
	for i, raw := range items {
		if limit > 0 && i >= limit {
			return nil
		}
		p, err := decodePaper(raw, resp)
		if err != nil {
			return err
		}
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}

// DailyPapers walks the daily feed. Each element wraps the paper in a paper
// key, and the wrapper carries the submission edge.
func (c *Client) DailyPapers(ctx context.Context, date, sort string, limit int, emit func(*Paper) error) error {
	u := query(c.api("daily_papers"),
		"date", date,
		"sort", sort,
		"limit", pageLimit(limit, 1, 100),
	)
	return walk(ctx, c, u, limit, decodePaper, emit)
}

// decodePaper handles both the bare paper shape and the daily feed's wrapper.
func decodePaper(raw json.RawMessage, resp *Response) (*Paper, error) {
	var env struct {
		Paper json.RawMessage `json:"paper"`
		// The daily wrapper carries these outside the paper object.
		NumComments           int       `json:"numComments"`
		Upvotes               int       `json:"upvotes"`
		PublishedAt           time.Time `json:"publishedAt"`
		Thumbnail             string    `json:"thumbnail"`
		IsAuthorParticipating bool      `json:"isAuthorParticipating"`
	}
	body := raw
	if jsonUnmarshal(raw, &env) == nil && len(env.Paper) > 0 {
		body = env.Paper
	}
	var p Paper
	if err := jsonUnmarshal(body, &p); err != nil {
		return nil, err
	}
	if p.NumComments == 0 {
		p.NumComments = env.NumComments
	}
	if p.Upvotes == 0 {
		p.Upvotes = env.Upvotes
	}
	if p.Thumbnail == "" {
		p.Thumbnail = env.Thumbnail
	}
	p.IsDaily = p.IsDaily || !p.SubmittedOnDailyAt.IsZero()
	p.normalize(resp.URL)
	return &p, nil
}

// Posts walks the social post feed.
func (c *Client) Posts(ctx context.Context, limit int, emit func(*Post) error) error {
	u := query(c.api("posts"), "limit", pageLimit(limit, 1, 100))
	return walkWrapped(ctx, c, u, "socialPosts", limit, decodePost, emit)
}

// Post fetches one post by its author and slug.
func (c *Client) Post(ctx context.Context, id string) (*Post, error) {
	user, slug, ok := strings.Cut(strings.Trim(id, "/"), "/")
	if !ok {
		return nil, errs.Usage("a post id is user/slug, got %q", id)
	}
	resp, err := c.Get(ctx, c.api("posts", user, slug))
	if err != nil {
		return nil, err
	}
	return decodePost(resp.Body, resp)
}

func decodePost(raw json.RawMessage, resp *Response) (*Post, error) {
	// The single-post route wraps the record in a post key, the feed does not.
	var env struct {
		Post json.RawMessage `json:"post"`
	}
	body := raw
	if jsonUnmarshal(raw, &env) == nil && len(env.Post) > 0 {
		body = env.Post
	}
	var p Post
	if err := jsonUnmarshal(body, &p); err != nil {
		return nil, err
	}
	p.normalize(resp.URL)
	return &p, nil
}

// rssFeed is the shape of /blog/feed.xml, which is the only index the blog has.
type rssFeed struct {
	Items []rssItem `xml:"channel>item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

// BlogFeed reads the RSS index. There is no blog JSON API at all, so this plus
// the page is the whole surface.
func (c *Client) BlogFeed(ctx context.Context, limit int, emit func(*BlogPost) error) error {
	u := c.Base + "/blog/feed.xml"
	resp, err := c.Get(ctx, u)
	if err != nil {
		return err
	}
	var feed rssFeed
	if err := xml.Unmarshal(resp.Body, &feed); err != nil {
		return errs.Wrap(errs.KindNetwork, err, "decode %s", shortURL(u))
	}
	for i, item := range feed.Items {
		if limit > 0 && i >= limit {
			return nil
		}
		post := &BlogPost{
			Title:       item.Title,
			Description: strings.TrimSpace(item.Description),
			GUID:        item.GUID,
		}
		if t, perr := time.Parse(time.RFC1123Z, item.PubDate); perr == nil {
			post.PublishedAt = t
		} else if t, perr := time.Parse(time.RFC1123, item.PubDate); perr == nil {
			post.PublishedAt = t
		}
		if _, slug, cerr := Classify(item.Link); cerr == nil {
			post.Slug = slug
		}
		post.setMeta(KindBlog, post.Slug, resp.URL)
		if post.URL == "" {
			post.URL = item.Link
		}
		if err := emit(post); err != nil {
			return err
		}
	}
	return nil
}

// Whoami reports who the current token belongs to. It is the only endpoint that
// requires one, and the only place the tool reports on its caller.
func (c *Client) Whoami(ctx context.Context) (*Whoami, error) {
	if c.Token == "" {
		return nil, errs.NeedAuth("whoami needs a token: set HF_TOKEN or pass --token")
	}
	var w Whoami
	resp, err := c.GetJSON(ctx, c.api("whoami-v2"), &w)
	if err != nil {
		return nil, err
	}
	for i := range w.Orgs {
		w.Orgs[i].Type = KindOrg
		w.Orgs[i].normalize()
	}
	kind := KindUser
	if w.Type == "org" {
		kind = KindOrg
	}
	w.setMeta(kind, w.Name, resp.URL)
	return &w, nil
}
