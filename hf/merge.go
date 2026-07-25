package hf

import (
	"context"
	"encoding/json"
)

// merge.go folds the page plane into records. The rule throughout is the one
// from the spec: a more specific source wins, and a present value never loses
// to an absent one. So every merge here reads "if the record does not have it
// and the page does, take it", and the few genuine exceptions are commented
// where they happen.

type deepKey struct{}

// WithDeep asks for the page plane on one call chain even when --deep is off.
// A couple of commands, upvoters most of all, have no answer anywhere else, and
// it is better for them to say so per call than to hand out a second client
// whose pacer has forgotten about the first one.
func WithDeep(ctx context.Context) context.Context {
	return context.WithValue(ctx, deepKey{}, true)
}

// deep is the gate every merge below asks. The flag is the usual answer and the
// context is the override.
func (c *Client) deep(ctx context.Context) bool {
	if c.Deep {
		return true
	}
	on, _ := ctx.Value(deepKey{}).(bool)
	return on
}

// DeepModel fetches a model and merges its page.
func (c *Client) DeepModel(ctx context.Context, id, rev string) (*Model, error) {
	m, err := c.Model(ctx, id, rev)
	if err != nil {
		return nil, err
	}
	if !c.deep(ctx) {
		return m, nil
	}
	p, err := c.PageOf(ctx, KindModel, m.ID)
	if err != nil {
		return m, nil
	}
	m.MergePage(p)
	return m, nil
}

// modelHeader is the payload of the ModelHeader component. Its model object
// carries a dozen fields no expand setting on /api/models returns.
type modelHeader struct {
	Model struct {
		TagObjs                     []Tag              `json:"tag_objs"`
		LibrariesOther              []string           `json:"librariesOther"`
		Region                      string             `json:"region"`
		IsQuantized                 bool               `json:"isQuantized"`
		LicenseFilePath             string             `json:"licenseFilePath"`
		CardExists                  bool               `json:"cardExists"`
		DiscussionsDisabled         bool               `json:"discussionsDisabled"`
		DiscussionsSorting          string             `json:"discussionsSorting"`
		HasBlockedOids              bool               `json:"hasBlockedOids"`
		TrackDownloads              bool               `json:"trackDownloads"`
		ShowHuggingChatEntry        bool               `json:"showHuggingChatEntry"`
		AvailableInferenceProviders InferenceProviders `json:"availableInferenceProviders"`
		Inference                   string             `json:"inference"`
	} `json:"model"`
	Author           *UserRef         `json:"author"`
	DiscussionsStats *DiscussionStats `json:"discussionsStats"`
	HasQuantizations bool             `json:"hasQuantizations"`
}

// MergePage folds a model page into the record.
func (m *Model) MergePage(p *Page) {
	if p == nil {
		return
	}
	m.addSource(p.URL)

	var h modelHeader
	if p.Into("ModelHeader", &h) {
		hm := h.Model
		// tag_objs has no API source at all, so the page always wins for it.
		if len(hm.TagObjs) > 0 {
			m.TagObjs = hm.TagObjs
			for i := range m.TagObjs {
				m.TagObjs[i].normalize(p.URL)
			}
		}
		takeStrings(&m.LibrariesOther, hm.LibrariesOther)
		takeString(&m.Region, hm.Region)
		takeString(&m.LicenseFilePath, hm.LicenseFilePath)
		takeString(&m.DiscussionsSorting, hm.DiscussionsSorting)
		m.IsQuantized = m.IsQuantized || hm.IsQuantized
		m.HasQuantizations = m.HasQuantizations || h.HasQuantizations
		m.CardExists = m.CardExists || hm.CardExists
		m.DiscussionsDisabled = m.DiscussionsDisabled || hm.DiscussionsDisabled
		m.HasBlockedOIDs = m.HasBlockedOIDs || hm.HasBlockedOids
		m.TrackDownloads = m.TrackDownloads || hm.TrackDownloads
		m.ShowHuggingChat = m.ShowHuggingChat || hm.ShowHuggingChatEntry
		if len(m.InferenceProviders) == 0 {
			m.InferenceProviders = hm.AvailableInferenceProviders
		}
		if h.Author != nil {
			// The page author object answers the user-or-org question with no
			// extra probe, which is why deep resolution is cheaper than it looks
			// for graph work.
			h.Author.normalize()
			m.AuthorData = h.Author
		}
		// Counts the page splits and the API totals are merged, not overwritten.
		if h.DiscussionsStats != nil {
			if m.DiscussionsStats == nil {
				m.DiscussionsStats = h.DiscussionsStats
			} else {
				if m.DiscussionsStats.Open == 0 {
					m.DiscussionsStats.Open = h.DiscussionsStats.Open
				}
				if m.DiscussionsStats.Closed == 0 {
					m.DiscussionsStats.Closed = h.DiscussionsStats.Closed
				}
				if m.DiscussionsStats.Total == 0 {
					m.DiscussionsStats.Total = h.DiscussionsStats.Total
				}
			}
		}
	}

	// LinkedSpacesList carries running and featured, which exist nowhere else.
	var spaces struct {
		LinkedSpaces []LinkedSpace `json:"linkedSpaces"`
	}
	if p.Into("LinkedSpacesList", &spaces) && len(spaces.LinkedSpaces) > 0 {
		m.LinkedSpaces = spaces.LinkedSpaces
	}

	var nav struct {
		TitleTree json.RawMessage `json:"titleTree"`
	}
	if p.Into("SideNavigation", &nav) && len(nav.TitleTree) > 0 {
		m.CardOutline = nav.TitleTree
	}

	takeString(&m.Thumbnail, p.Thumbnail())
}

// DeepDataset fetches a dataset and merges its page. The page's DatasetViewer
// payload is one request where the viewer API would be three.
func (c *Client) DeepDataset(ctx context.Context, id, rev string) (*Dataset, error) {
	d, err := c.Dataset(ctx, id, rev)
	if err != nil {
		return nil, err
	}
	if !c.deep(ctx) {
		return d, nil
	}
	p, err := c.PageOf(ctx, KindDataset, d.ID)
	if err != nil {
		return d, nil
	}
	d.MergePage(p)
	return d, nil
}

// MergePage folds a dataset page into the record.
func (d *Dataset) MergePage(p *Page) {
	if p == nil {
		return
	}
	d.addSource(p.URL)

	var h struct {
		Dataset struct {
			TagObjs             []Tag  `json:"tag_objs"`
			Region              string `json:"region"`
			CardExists          bool   `json:"cardExists"`
			DiscussionsDisabled bool   `json:"discussionsDisabled"`
			HasBlockedOids      bool   `json:"hasBlockedOids"`
			LicenseFilePath     string `json:"licenseFilePath"`
		} `json:"dataset"`
		Author           *UserRef         `json:"author"`
		DiscussionsStats *DiscussionStats `json:"discussionsStats"`
	}
	if p.Into("DatasetHeader", &h) {
		if len(h.Dataset.TagObjs) > 0 {
			d.TagObjs = h.Dataset.TagObjs
			for i := range d.TagObjs {
				d.TagObjs[i].normalize(p.URL)
			}
		}
		takeString(&d.Region, h.Dataset.Region)
		takeString(&d.LicenseFilePath, h.Dataset.LicenseFilePath)
		d.CardExists = d.CardExists || h.Dataset.CardExists
		d.DiscussionsDisabled = d.DiscussionsDisabled || h.Dataset.DiscussionsDisabled
		d.HasBlockedOIDs = d.HasBlockedOIDs || h.Dataset.HasBlockedOids
		if h.Author != nil {
			h.Author.normalize()
			d.AuthorData = h.Author
		}
		if d.DiscussionsStats == nil {
			d.DiscussionsStats = h.DiscussionsStats
		}
	}

	var viewer struct {
		HasParquetFormat bool            `json:"hasParquetFormat"`
		IsTracesDataset  bool            `json:"isTracesDataset"`
		Data             json.RawMessage `json:"data"`
	}
	if p.Into("DatasetViewer", &viewer) {
		d.HasParquetFormat = d.HasParquetFormat || viewer.HasParquetFormat
		d.IsTracesDataset = d.IsTracesDataset || viewer.IsTracesDataset
		d.ViewerData = viewer.Data
	}

	var libs struct {
		Libraries []DatasetLib `json:"libraries"`
	}
	if p.Into("DatasetLibrary", &libs) && len(libs.Libraries) > 0 {
		d.Libraries = libs.Libraries
	}

	var spaces struct {
		LinkedSpaces []LinkedSpace `json:"linkedSpaces"`
	}
	if p.Into("LinkedSpacesList", &spaces) && len(spaces.LinkedSpaces) > 0 {
		d.LinkedSpaces = spaces.LinkedSpaces
	}

	if len(p.LD) > 0 {
		d.JSONLD = p.LD[0]
	}
	takeString(&d.Thumbnail, p.Thumbnail())
}

// DeepSpace fetches a space and merges its page. SpaceHeader.space carries the
// runtime state inline, so this avoids the separate /runtime call.
func (c *Client) DeepSpace(ctx context.Context, id, rev string) (*Space, error) {
	s, err := c.Space(ctx, id, rev)
	if err != nil {
		return nil, err
	}
	if !c.deep(ctx) {
		return s, nil
	}
	p, err := c.PageOf(ctx, KindSpace, s.ID)
	if err != nil {
		return s, nil
	}
	s.MergePage(p)
	return s, nil
}

// MergePage folds a space page into the record.
func (s *Space) MergePage(p *Page) {
	if p == nil {
		return
	}
	s.addSource(p.URL)

	var h struct {
		Space struct {
			TagObjs []Tag    `json:"tag_objs"`
			Region  string   `json:"region"`
			Runtime *Runtime `json:"runtime"`
		} `json:"space"`
		Author           *UserRef         `json:"author"`
		DiscussionsStats *DiscussionStats `json:"discussionsStats"`
	}
	if p.Into("SpaceHeader", &h) {
		if len(h.Space.TagObjs) > 0 {
			s.TagObjs = h.Space.TagObjs
			for i := range s.TagObjs {
				s.TagObjs[i].normalize(p.URL)
			}
		}
		takeString(&s.Region, h.Space.Region)
		// The dedicated /runtime endpoint beats this, so the page only fills a
		// gap it did not already cover.
		if s.Runtime == nil {
			s.Runtime = h.Space.Runtime
		}
		if h.Author != nil {
			h.Author.normalize()
			s.AuthorData = h.Author
		}
		if s.DiscussionsStats == nil {
			s.DiscussionsStats = h.DiscussionsStats
		}
	}

	var inner struct {
		IFrameSrc          string `json:"iframeSrc"`
		ShowGettingStarted bool   `json:"showGettingStarted"`
		CanRestart         bool   `json:"canRestart"`
	}
	if p.Into("SpacePageInner", &inner) {
		takeString(&s.IframeSrc, inner.IFrameSrc)
		s.ShowGettingStarted = s.ShowGettingStarted || inner.ShowGettingStarted
	}

	if len(p.LD) > 0 {
		s.JSONLD = p.LD[0]
	}
	takeString(&s.ShortDescription, p.Description())
	takeString(&s.Thumbnail, p.Thumbnail())
}

// DeepOrg fetches an org page, which is the biggest single win in the tool:
// one request replaces the overview, the members, a follower sample, three list
// calls, a collections call, and a papers call.
func (c *Client) DeepOrg(ctx context.Context, name string) (*Org, error) {
	o, err := c.Org(ctx, name)
	if err != nil {
		return nil, err
	}
	if !c.deep(ctx) {
		return o, nil
	}
	p, err := c.PageOf(ctx, KindOrg, name)
	if err != nil {
		return o, nil
	}
	o.MergePage(p)
	return o, nil
}

// MergePage folds an org page into the record.
func (o *Org) MergePage(p *Page) {
	if p == nil {
		return
	}
	o.addSource(p.URL)

	var prof struct {
		OrganizationCard string       `json:"organizationCard"`
		Users            []UserRef    `json:"users"`
		UserCount        int          `json:"userCount"`
		Models           []Model      `json:"models"`
		Datasets         []Dataset    `json:"datasets"`
		Spaces           []Space      `json:"spaces"`
		Collections      []Collection `json:"collections"`
		PaperPreviews    []Paper      `json:"paperPreviews"`
	}
	if p.Into("OrgProfile", &prof) {
		takeString(&o.Card, prof.OrganizationCard)
		if len(prof.Users) > 0 {
			o.Members = prof.Users
			for i := range o.Members {
				o.Members[i].normalize()
			}
		}
		if o.NumUsers == 0 {
			o.NumUsers = prof.UserCount
		}
		o.Models = normalizeAll(prof.Models, KindModel, p.URL, func(m *Model) *Repo { return &m.Repo })
		o.Datasets = normalizeAll(prof.Datasets, KindDataset, p.URL, func(d *Dataset) *Repo { return &d.Repo })
		o.Spaces = normalizeAll(prof.Spaces, KindSpace, p.URL, func(s *Space) *Repo { return &s.Repo })
		for i := range prof.Collections {
			prof.Collections[i].normalize(p.URL)
		}
		o.Collections = prof.Collections
		for i := range prof.PaperPreviews {
			prof.PaperPreviews[i].normalize(p.URL)
		}
		o.Papers = prof.PaperPreviews
	}

	var actions struct {
		FollowerCount   int       `json:"followerCount"`
		SampleFollowers []UserRef `json:"sampleFollowers"`
	}
	if p.Into("OrgHeaderActions", &actions) {
		if o.NumFollowers == 0 {
			o.NumFollowers = actions.FollowerCount
		}
		if len(actions.SampleFollowers) > 0 {
			o.SampleFollowers = actions.SampleFollowers
			for i := range o.SampleFollowers {
				o.SampleFollowers[i].normalize()
			}
		}
	}
	takeString(&o.AvatarURL, p.Thumbnail())
}

// DeepUser fetches a user page, whose activity feed exists nowhere in the API.
func (c *Client) DeepUser(ctx context.Context, name string) (*User, error) {
	u, err := c.User(ctx, name)
	if err != nil {
		return nil, err
	}
	if !c.deep(ctx) {
		return u, nil
	}
	p, err := c.PageOf(ctx, KindUser, name)
	if err != nil {
		return u, nil
	}
	u.MergePage(p)
	return u, nil
}

// MergePage folds a user page into the record.
func (u *User) MergePage(p *Page) {
	if p == nil {
		return
	}
	u.addSource(p.URL)

	var prof struct {
		Models             []Model      `json:"models"`
		Datasets           []Dataset    `json:"datasets"`
		Spaces             []Space      `json:"spaces"`
		Collections        []Collection `json:"collections"`
		BlogPosts          []BlogRef    `json:"blogPosts"`
		TotalBlogPosts     int          `json:"totalBlogPosts"`
		LastUserActivities []Activity   `json:"lastUserActivities"`
		CommunityScore     int          `json:"communityScore"`
	}
	if p.Into("UserProfile", &prof) {
		u.Models = normalizeAll(prof.Models, KindModel, p.URL, func(m *Model) *Repo { return &m.Repo })
		u.Datasets = normalizeAll(prof.Datasets, KindDataset, p.URL, func(d *Dataset) *Repo { return &d.Repo })
		u.Spaces = normalizeAll(prof.Spaces, KindSpace, p.URL, func(s *Space) *Repo { return &s.Repo })
		for i := range prof.Collections {
			prof.Collections[i].normalize(p.URL)
		}
		u.Collections = prof.Collections
		u.BlogPosts = prof.BlogPosts
		for i := range u.BlogPosts {
			u.BlogPosts[i].URI = URI(KindBlog, u.BlogPosts[i].Slug)
			u.BlogPosts[i].URL = BaseURL + "/blog/" + u.BlogPosts[i].Slug
		}
		if u.TotalBlogPosts == 0 {
			u.TotalBlogPosts = prof.TotalBlogPosts
		}
		// The activity feed is a timestamped edge stream and the closest thing
		// the hub has to an event log, so it is kept whole.
		u.Activities = prof.LastUserActivities
		if u.CommunityScore == 0 {
			u.CommunityScore = prof.CommunityScore
		}
	}
	takeString(&u.AvatarURL, p.Thumbnail())
}

// DeepPaper fetches a paper and merges its page, which carries the upvotes, the
// AI summary, the comment thread, and the markdown body link the API omits.
func (c *Client) DeepPaper(ctx context.Context, id string) (*Paper, error) {
	pa, err := c.Paper(ctx, id)
	if err != nil {
		return nil, err
	}
	if !c.deep(ctx) {
		return pa, nil
	}
	p, err := c.PageOf(ctx, KindPaper, pa.ID)
	if err != nil {
		return pa, nil
	}
	pa.MergePage(p)
	return pa, nil
}

// MergePage folds a paper page into the record.
func (pa *Paper) MergePage(p *Page) {
	if p == nil {
		return
	}
	pa.addSource(p.URL)

	var content struct {
		Paper struct {
			Upvotes        int      `json:"upvotes"`
			DiscussionID   string   `json:"discussionId"`
			AISummary      string   `json:"ai_summary"`
			AIKeywords     []string `json:"ai_keywords"`
			AISummaryModel string   `json:"ai_summary_model"`
			GitHubRepo     string   `json:"githubRepo"`
			ProjectPage    string   `json:"projectPage"`
		} `json:"paper"`
		Comments    []Comment `json:"comments"`
		Upvoters    []UserRef `json:"upvoters"`
		MarkdownURL string    `json:"markdownContentUrl"`
	}
	if p.Into("PaperContent", &content) {
		if pa.Upvotes == 0 {
			pa.Upvotes = content.Paper.Upvotes
		}
		takeString(&pa.DiscussionID, content.Paper.DiscussionID)
		takeString(&pa.AISummary, content.Paper.AISummary)
		takeString(&pa.AISummaryModel, content.Paper.AISummaryModel)
		takeString(&pa.GitHubRepo, content.Paper.GitHubRepo)
		takeString(&pa.ProjectPage, content.Paper.ProjectPage)
		takeStrings(&pa.AIKeywords, content.Paper.AIKeywords)
		// The markdown URL is the paper's full text, and no API route mentions
		// it at all.
		takeString(&pa.MarkdownURL, content.MarkdownURL)
		if len(content.Comments) > 0 {
			pa.Comments = content.Comments
			for i := range pa.Comments {
				if pa.Comments[i].Author != nil {
					pa.Comments[i].Author.normalize()
				}
			}
		}
		if len(content.Upvoters) > 0 {
			pa.Upvoters = content.Upvoters
			for i := range pa.Upvoters {
				pa.Upvoters[i].normalize()
			}
		}
	}

	var upvote struct {
		Upvotes  int       `json:"upvotes"`
		Upvoters []UserRef `json:"upvoters"`
	}
	if p.Into("UpvoteControl", &upvote) {
		if pa.Upvotes == 0 {
			pa.Upvotes = upvote.Upvotes
		}
		if len(pa.Upvoters) == 0 {
			pa.Upvoters = upvote.Upvoters
		}
	}

	var spaces struct {
		LinkedSpaces []LinkedSpace `json:"linkedSpaces"`
	}
	if p.Into("LinkedSpacesList", &spaces) {
		pa.LinkedSpaces = spaces.LinkedSpaces
		for _, s := range spaces.LinkedSpaces {
			pa.Spaces = append(pa.Spaces, s.ID)
		}
	}
	takeString(&pa.Thumbnail, p.Thumbnail())
}

// DeepCollection fetches a collection and merges its page.
func (c *Client) DeepCollection(ctx context.Context, slug string) (*Collection, error) {
	col, err := c.Collection(ctx, slug)
	if err != nil {
		return nil, err
	}
	if !c.deep(ctx) {
		return col, nil
	}
	p, err := c.PageOf(ctx, KindCollection, col.Slug)
	if err != nil {
		return col, nil
	}
	col.MergePage(p)
	return col, nil
}

// MergePage folds a collection page into the record.
func (col *Collection) MergePage(p *Page) {
	if p == nil {
		return
	}
	col.addSource(p.URL)
	var payload struct {
		Upvoters []UserRef `json:"upvoters"`
	}
	if p.Into("Collection", &payload) && len(payload.Upvoters) > 0 {
		col.Upvoters = payload.Upvoters
		for i := range col.Upvoters {
			col.Upvoters[i].normalize()
		}
	}
}

// DeepBlog reads a blog post, which has no JSON API at all: the byline,
// thumbnail, upvotes, and comments come from payloads, and the body from the
// markup.
func (c *Client) DeepBlog(ctx context.Context, slug string) (*BlogPost, error) {
	p, err := c.PageOf(ctx, KindBlog, slug)
	if err != nil {
		return nil, err
	}
	post := &BlogPost{Slug: slug}
	post.MergePage(p)
	return post, nil
}

// MergePage builds a blog post out of its page.
func (b *BlogPost) MergePage(p *Page) {
	if p == nil {
		return
	}

	var byline struct {
		Authors      []UserRef `json:"authors"`
		Translators  []UserRef `json:"translators"`
		Proofreaders []UserRef `json:"proofreaders"`
		Lang         string    `json:"lang"`
	}
	if p.Into("BlogAuthorsByline", &byline) {
		b.Authors = normalizeRefs(byline.Authors)
		b.Translators = normalizeRefs(byline.Translators)
		b.Proofreaders = normalizeRefs(byline.Proofreaders)
		takeString(&b.Lang, byline.Lang)
	}

	var thumb struct {
		Blog struct {
			Title       string   `json:"title"`
			Slug        string   `json:"slug"`
			Thumbnail   string   `json:"thumbnail"`
			PublishedAt Time     `json:"date"`
			Tags        []string `json:"tags"`
		} `json:"blog"`
	}
	if p.Into("BlogThumbnail", &thumb) {
		takeString(&b.Title, thumb.Blog.Title)
		takeString(&b.Slug, thumb.Blog.Slug)
		takeString(&b.Thumbnail, thumb.Blog.Thumbnail)
		takeStrings(&b.Tags, thumb.Blog.Tags)
		if b.PublishedAt.IsZero() {
			b.PublishedAt = thumb.Blog.PublishedAt.Time
		}
	}

	var upvote struct {
		Upvotes  int       `json:"upvotes"`
		Upvoters []UserRef `json:"upvoters"`
	}
	if p.Into("UpvoteControl", &upvote) {
		if b.Upvotes == 0 {
			b.Upvotes = upvote.Upvotes
		}
		if len(b.Upvoters) == 0 {
			b.Upvoters = normalizeRefs(upvote.Upvoters)
		}
	}

	var events struct {
		Discussion struct {
			Events []DiscussionEvent `json:"events"`
		} `json:"discussion"`
	}
	if p.Into("DiscussionEvents", &events) {
		for _, e := range events.Discussion.Events {
			e.normalize()
			if e.Comment != nil {
				b.Comments = append(b.Comments, *e.Comment)
			}
		}
	}

	takeString(&b.Title, p.Title)
	takeString(&b.Description, p.Description())
	takeString(&b.Thumbnail, p.Thumbnail())
	b.Blocks = ExtractBlocks(p.HTML)
	if b.Body == "" {
		b.Body = BlocksToMarkdown(b.Blocks)
	}
	b.setMeta(KindBlog, b.Slug, p.URL)
}

// DeepDiscussion fetches a thread's page, which also resolves the parent repo
// in the same request because the repo header renders on it.
func (c *Client) DeepDiscussion(ctx context.Context, kind, id string, num int) (*Discussion, error) {
	d, err := c.Discussion(ctx, kind, id, num)
	if err != nil {
		return nil, err
	}
	if !c.deep(ctx) {
		return d, nil
	}
	p, err := c.PageOf(ctx, KindDiscussion, DiscussionID(kind, id, num))
	if err != nil {
		return d, nil
	}
	d.MergePage(p)
	return d, nil
}

// MergePage folds a discussion page into the record.
func (d *Discussion) MergePage(p *Page) {
	if p == nil {
		return
	}
	d.addSource(p.URL)
	var events struct {
		Discussion struct {
			Events []DiscussionEvent `json:"events"`
			Diff   string            `json:"diff"`
		} `json:"discussion"`
	}
	if p.Into("DiscussionEvents", &events) {
		if len(events.Discussion.Events) > 0 {
			d.Events = events.Discussion.Events
			for i := range d.Events {
				d.Events[i].normalize()
			}
		}
		takeString(&d.Diff, events.Discussion.Diff)
	}
}

// --- merge helpers ---
//
// These exist so the "a present value never loses to an absent one" rule reads
// the same everywhere rather than being spelled out at every field.

func takeString(dst *string, v string) {
	if *dst == "" {
		*dst = v
	}
}

func takeStrings(dst *[]string, v []string) {
	if len(*dst) == 0 {
		*dst = v
	}
}

func normalizeRefs(refs []UserRef) []UserRef {
	for i := range refs {
		refs[i].normalize()
	}
	return refs
}

// normalizeAll addresses a slice of repo stubs lifted off a profile page.
func normalizeAll[T any](items []T, kind, sourceURL string, repo func(*T) *Repo) []T {
	for i := range items {
		repo(&items[i]).normalize(kind, sourceURL)
	}
	return items
}
