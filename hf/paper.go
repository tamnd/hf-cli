package hf

import "time"

// Paper is a piece of literature the hub tracks, keyed by its arXiv id. Papers
// are what connect the hub to the research it implements, and the arxiv: tag on
// a repo is the edge in the other direction.
type Paper struct {
	Meta

	ID          string        `json:"id"`
	Title       string        `json:"title"`
	Summary     string        `json:"summary,omitempty"`
	PublishedAt time.Time     `json:"publishedAt,omitzero"`
	Authors     []PaperAuthor `json:"authors,omitempty"`

	// Daily feed fields.
	SubmittedOnDailyAt time.Time `json:"submittedOnDailyAt,omitzero"`
	SubmittedOnDailyBy *UserRef  `json:"submittedOnDailyBy,omitempty"`
	IsDaily            bool      `json:"isDaily,omitempty"`

	// Organization is the lab the hub credits for the work. It is the only place
	// on the site a paper is attributed to an institution rather than to the
	// people who submitted it.
	Organization *UserRef `json:"organization,omitempty"`
	MediaURLs    []string `json:"mediaUrls,omitempty"`

	// GitHubStars is a count the hub scraped, so it lags the repo it describes.
	// GitHubRepoAddedBy says who linked the repo, and reads "user" rather than a
	// name when it was the community rather than an author.
	GitHubStars       int    `json:"githubStars,omitempty"`
	GitHubRepoAddedBy string `json:"githubRepoAddedBy,omitempty"`

	// Page-derived.
	Upvotes        int           `json:"upvotes,omitempty"`
	NumComments    int           `json:"numComments,omitempty"`
	DiscussionID   string        `json:"discussionId,omitempty"`
	AISummary      string        `json:"ai_summary,omitempty"`
	AIKeywords     []string      `json:"ai_keywords,omitempty"`
	AISummaryModel string        `json:"ai_summary_model,omitempty"`
	MarkdownURL    string        `json:"markdownContentUrl,omitempty"`
	Upvoters       []UserRef     `json:"upvoters,omitempty"`
	Comments       []Comment     `json:"comments,omitempty"`
	LinkedSpaces   []LinkedSpace `json:"linkedSpaces,omitempty"`
	GitHubRepo     string        `json:"githubRepo,omitempty"`
	ProjectPage    string        `json:"projectPage,omitempty"`
	Thumbnail      string        `json:"thumbnail,omitempty"`

	// The detail endpoint carries the first page of each linked repo list inline
	// along with the totals, so one request answers what three searches would and
	// says how much more there is.
	LinkedModels     []Model   `json:"linkedModels,omitempty"`
	LinkedDatasets   []Dataset `json:"linkedDatasets,omitempty"`
	NumTotalModels   int       `json:"numTotalModels,omitempty"`
	NumTotalDatasets int       `json:"numTotalDatasets,omitempty"`
	NumTotalSpaces   int       `json:"numTotalSpaces,omitempty"`

	// Derived by hf rather than read.
	ArxivURL string   `json:"arxivUrl,omitempty"`
	Models   []string `json:"models,omitempty"`
	Datasets []string `json:"datasets,omitempty"`
	Spaces   []string `json:"spaces,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (p *Paper) UnmarshalJSON(b []byte) error {
	type raw Paper
	return decodeExtra(b, (*raw)(p), &p.Extra)
}

func (p *Paper) normalize(sourceURL string) {
	if p.ID != "" {
		p.ArxivURL = "https://arxiv.org/abs/" + p.ID
	}
	if p.SubmittedOnDailyBy != nil {
		p.SubmittedOnDailyBy.normalize()
	}
	if p.Organization != nil {
		// The organization object omits the type, and an institution credited for
		// a paper is always an org, so say so rather than leaving it ambiguous.
		if p.Organization.Type == "" {
			p.Organization.Type = "org"
		}
		p.Organization.normalize()
	}
	for i := range p.Authors {
		if p.Authors[i].User != nil {
			p.Authors[i].User.normalize()
		}
	}
	for i := range p.LinkedModels {
		p.LinkedModels[i].normalize(KindModel, sourceURL)
	}
	for i := range p.LinkedDatasets {
		p.LinkedDatasets[i].normalize(KindDataset, sourceURL)
	}
	p.setMeta(KindPaper, p.ID, sourceURL)
}

// PaperAuthor is a name, and sometimes a claimed hub account. An author with a
// verified status is one of the few edges on the site a human explicitly
// confirmed, which is why it gets its own predicate in the graph.
type PaperAuthor struct {
	ObjectID            string    `json:"_id,omitempty"`
	Name                string    `json:"name"`
	Hidden              bool      `json:"hidden,omitempty"`
	Status              string    `json:"status,omitempty"`
	StatusLastChangedAt time.Time `json:"statusLastChangedAt,omitzero"`
	User                *UserRef  `json:"user,omitempty"`
}

// Verified reports whether this author claimed the paper and the claim was
// accepted. An unverified name match is not an identity and does not become an
// edge.
func (a PaperAuthor) Verified() bool {
	return a.User != nil && a.Status == "claimed_verified"
}
