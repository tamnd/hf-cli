package hf

import (
	"context"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// ops.go is the table of contents for the whole tool. Every verb a person can
// type is registered here and nowhere else, so the answer to "what can hf do"
// is one file, and each registration is simultaneously a CLI subcommand, an
// HTTP route under `hf serve`, and an MCP tool under `hf mcp`.
//
// The handlers are thin on purpose. Anything with a decision in it belongs in
// the library next to the data it decides about; what is left here is argument
// resolution and one call.

func registerOps(app *kit.App) {
	registerReadOps(app)
	registerListOps(app)
	registerRelationOps(app)
	registerRepoOps(app)
	registerDataOps(app)
	registerGraphOps(app)
}

// --- reference resolution ---

// ResolveRef resolves any accepted reference to the id for the kind a command names.
//
// Classify has two defaults: one bare segment is a namespace, two are a model.
// Those are guesses, and a command that names its own kind knows better, so a
// guess yields to the command while an explicit URL or URI does not. That is
// what makes `hf dataset rajpurkar/squad` work and `hf dataset
// https://huggingface.co/google/gemma-7b` fail with a useful message.
func ResolveRef(want, input string) (string, error) {
	kind, id, err := Classify(input)
	if err != nil {
		return "", err
	}
	if kind == want {
		return id, nil
	}
	guessed := !strings.Contains(input, "://") && (kind == KindModel || kind == KindNamespace)
	if guessed {
		return id, nil
	}
	// A sub-resource names its repo, so a file or discussion URL is a fine way
	// to refer to the repo it lives in.
	switch kind {
	case KindDiscussion:
		if k, repo, _, ok := splitDiscussionID(id); ok && k == want {
			return repo, nil
		}
	case KindFile, KindCommit, KindRef:
		if k, repo, _, _, ok := splitRevID(id); ok && k == want {
			return repo, nil
		}
	}
	return "", errs.Usage("%q is a %s, not a %s", input, kind, want)
}

// ResolveRepo resolves a reference to any of the four repository kinds, keeping the
// kind the reference named. Commands that work on all four use it so that
// `hf tree` and `hf likers` do not each need a --type flag.
func ResolveRepo(input string) (kind, id string, err error) {
	kind, id, err = Classify(input)
	if err != nil {
		return "", "", err
	}
	switch kind {
	case KindModel, KindDataset, KindSpace, KindKernel:
		return kind, id, nil
	case KindNamespace:
		return KindModel, id, nil
	case KindDiscussion:
		if k, repo, _, ok := splitDiscussionID(id); ok {
			return k, repo, nil
		}
	case KindFile, KindCommit, KindRef:
		if k, repo, _, _, ok := splitRevID(id); ok {
			return k, repo, nil
		}
	}
	return "", "", errs.Usage("%q is a %s, not a repository", input, kind)
}

// emitEach sends a slice one record at a time. The client methods that return a
// slice do so because upstream answers one document; the command surface still
// wants records, and a whole document is never large enough to be worth
// streaming through a channel.
func emitEach[T any](items []T, emit func(*T) error) error {
	for i := range items {
		if err := emit(&items[i]); err != nil {
			return err
		}
	}
	return nil
}

// --- reading one thing ---

// refIn is the input every single-entity read shares.
type refIn struct {
	C   *Client `kit:"inject"`
	Ref string  `kit:"arg" help:"an id, a hub URL, or an hf:// URI"`
	Rev string  `kit:"flag" help:"branch, tag, or commit sha"`
}

// bareRefIn is the input for the commands that take a reference and nothing
// else. It is separate from refIn because a --rev the command would ignore is
// worse than no flag at all.
type bareRefIn struct {
	C   *Client `kit:"inject"`
	Ref string  `kit:"arg" help:"an id, a hub URL, or an hf:// URI"`
}

// nameIn is the input for the entities addressed by a bare name rather than a
// repository path.
type nameIn struct {
	C    *Client `kit:"inject"`
	Name string  `kit:"arg" help:"a name, a hub URL, or an hf:// URI"`
}

func registerReadOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "model", Group: "read", Single: true, URIType: KindModel, Resolver: true,
		Summary: "Read one model with every field the hub returns",
		Args:    []kit.Arg{{Name: "ref", Help: "model id, URL, or hf:// URI"}},
	}, getModel)

	kit.Handle(app, kit.OpMeta{
		Name: "dataset", Group: "read", Single: true, URIType: KindDataset, Resolver: true,
		Summary: "Read one dataset with every field the hub returns",
		Args:    []kit.Arg{{Name: "ref", Help: "dataset id, URL, or hf:// URI"}},
	}, getDataset)

	kit.Handle(app, kit.OpMeta{
		Name: "space", Group: "read", Single: true, URIType: KindSpace, Resolver: true,
		Summary: "Read one space, runtime state included",
		Args:    []kit.Arg{{Name: "ref", Help: "space id, URL, or hf:// URI"}},
	}, getSpace)

	kit.Handle(app, kit.OpMeta{
		Name: "kernel", Group: "read", Single: true, URIType: KindKernel, Resolver: true,
		Summary: "Read one compute kernel",
		Args:    []kit.Arg{{Name: "ref", Help: "kernel id, URL, or hf:// URI"}},
	}, getKernel)

	kit.Handle(app, kit.OpMeta{
		Name: "user", Group: "read", Single: true, URIType: KindUser, Resolver: true,
		Summary: "Read one user profile",
		Args:    []kit.Arg{{Name: "name", Help: "user name, URL, or @handle"}},
	}, getUser)

	kit.Handle(app, kit.OpMeta{
		Name: "org", Group: "read", Single: true, URIType: KindOrg, Resolver: true,
		Summary: "Read one organization",
		Args:    []kit.Arg{{Name: "name", Help: "org name, URL, or @handle"}},
	}, getOrg)

	kit.Handle(app, kit.OpMeta{
		Name: "ns", Group: "read", Single: true, URIType: KindNamespace, Resolver: true,
		Aliases: []string{"namespace"},
		Summary: "Read a name without knowing whether it is a user or an org",
		Args:    []kit.Arg{{Name: "name", Help: "user or org name"}},
	}, getNamespace)

	kit.Handle(app, kit.OpMeta{
		Name: "collection", Group: "read", Single: true, URIType: KindCollection, Resolver: true,
		Summary: "Read one collection and its items",
		Args:    []kit.Arg{{Name: "slug", Help: "collection slug or URL"}},
	}, getCollection)

	kit.Handle(app, kit.OpMeta{
		Name: "paper", Group: "read", Single: true, URIType: KindPaper, Resolver: true,
		Summary: "Read one paper, its authors, and what cites it",
		Args:    []kit.Arg{{Name: "id", Help: "arXiv id, arXiv URL, or hub paper URL"}},
	}, getPaper)

	kit.Handle(app, kit.OpMeta{
		Name: "post", Group: "read", Single: true, URIType: KindPost, Resolver: true,
		Summary: "Read one community post",
		Args:    []kit.Arg{{Name: "ref", Help: "author/slug or post URL"}},
	}, getPost)

	kit.Handle(app, kit.OpMeta{
		Name: "blog", Group: "read", Single: true, URIType: KindBlog, Resolver: true,
		Summary: "Read one blog entry, body included",
		Args:    []kit.Arg{{Name: "slug", Help: "blog slug or URL"}},
	}, getBlog)

	kit.Handle(app, kit.OpMeta{
		Name: "discussion", Group: "read", Single: true, URIType: KindDiscussion, Resolver: true,
		Summary: "Read one discussion or pull request with its full event thread",
		Args: []kit.Arg{
			{Name: "repo", Help: "repository ref"},
			{Name: "num", Help: "discussion number"},
		},
	}, getDiscussion)

	kit.Handle(app, kit.OpMeta{
		Name: "whoami", Group: "read", Single: true,
		Summary: "Report who the current token belongs to",
	}, getWhoami)

	kit.Handle(app, kit.OpMeta{
		Name: "get", Group: "read", Single: true,
		Summary: "Read whatever a reference points at",
		Long: "get classifies the reference and dispatches to the right reader, which is\n" +
			"what makes `hf crawl ... -o url | xargs -n1 hf get` work across mixed kinds.",
		Args: []kit.Arg{{Name: "ref", Help: "any hub reference"}},
	}, getAny)
}

func getModel(ctx context.Context, in refIn, emit func(*Model) error) error {
	id, err := ResolveRef(KindModel, in.Ref)
	if err != nil {
		return err
	}
	m, err := in.C.DeepModel(ctx, id, in.Rev)
	if err != nil {
		return err
	}
	return emit(m)
}

func getDataset(ctx context.Context, in refIn, emit func(*Dataset) error) error {
	id, err := ResolveRef(KindDataset, in.Ref)
	if err != nil {
		return err
	}
	d, err := in.C.DeepDataset(ctx, id, in.Rev)
	if err != nil {
		return err
	}
	return emit(d)
}

func getSpace(ctx context.Context, in refIn, emit func(*Space) error) error {
	id, err := ResolveRef(KindSpace, in.Ref)
	if err != nil {
		return err
	}
	s, err := in.C.DeepSpace(ctx, id, in.Rev)
	if err != nil {
		return err
	}
	return emit(s)
}

func getKernel(ctx context.Context, in refIn, emit func(*Kernel) error) error {
	id, err := ResolveRef(KindKernel, in.Ref)
	if err != nil {
		return err
	}
	k, err := in.C.Kernel(ctx, id, in.Rev)
	if err != nil {
		return err
	}
	return emit(k)
}

func getUser(ctx context.Context, in nameIn, emit func(*User) error) error {
	id, err := ResolveRef(KindUser, in.Name)
	if err != nil {
		return err
	}
	u, err := in.C.DeepUser(ctx, id)
	if err != nil {
		return err
	}
	return emit(u)
}

func getOrg(ctx context.Context, in nameIn, emit func(*Org) error) error {
	id, err := ResolveRef(KindOrg, in.Name)
	if err != nil {
		return err
	}
	o, err := in.C.DeepOrg(ctx, id)
	if err != nil {
		return err
	}
	return emit(o)
}

// getNamespace answers for a name whose kind nobody knows yet. One HEAD-shaped
// probe decides, and the result is memoised on the client, so a pipeline that
// asks about the same owner repeatedly pays for the question once.
func getNamespace(ctx context.Context, in nameIn, emit func(any) error) error {
	id, err := ResolveRef(KindNamespace, in.Name)
	if err != nil {
		return err
	}
	kind, err := in.C.NamespaceKind(ctx, id)
	if err != nil {
		return err
	}
	rec, err := in.C.Fetch(ctx, kind, id)
	if err != nil {
		return err
	}
	return emit(rec)
}

func getCollection(ctx context.Context, in nameIn, emit func(*Collection) error) error {
	slug, err := ResolveRef(KindCollection, in.Name)
	if err != nil {
		return err
	}
	col, err := in.C.DeepCollection(ctx, slug)
	if err != nil {
		return err
	}
	return emit(col)
}

func getPaper(ctx context.Context, in nameIn, emit func(*Paper) error) error {
	id, err := ResolveRef(KindPaper, in.Name)
	if err != nil {
		return err
	}
	p, err := in.C.DeepPaper(ctx, id)
	if err != nil {
		return err
	}
	return emit(p)
}

func getPost(ctx context.Context, in nameIn, emit func(*Post) error) error {
	id, err := ResolveRef(KindPost, in.Name)
	if err != nil {
		return err
	}
	p, err := in.C.Post(ctx, id)
	if err != nil {
		return err
	}
	return emit(p)
}

func getBlog(ctx context.Context, in nameIn, emit func(*BlogPost) error) error {
	slug, err := ResolveRef(KindBlog, in.Name)
	if err != nil {
		return err
	}
	b, err := in.C.DeepBlog(ctx, slug)
	if err != nil {
		return err
	}
	return emit(b)
}

type discussionIn struct {
	C    *Client `kit:"inject"`
	Repo string  `kit:"arg" help:"repository ref"`
	Num  int     `kit:"arg" help:"discussion number"`
}

func getDiscussion(ctx context.Context, in discussionIn, emit func(*Discussion) error) error {
	kind, id, err := ResolveRepo(in.Repo)
	if err != nil {
		return err
	}
	d, err := in.C.DeepDiscussion(ctx, kind, id, in.Num)
	if err != nil {
		return err
	}
	return emit(d)
}

type clientIn struct {
	C *Client `kit:"inject"`
}

func getWhoami(ctx context.Context, in clientIn, emit func(*Whoami) error) error {
	w, err := in.C.Whoami(ctx)
	if err != nil {
		return err
	}
	return emit(w)
}

func getAny(ctx context.Context, in bareRefIn, emit func(any) error) error {
	_, _, rec, err := in.C.FetchRef(ctx, in.Ref)
	if err != nil {
		return err
	}
	return emit(rec)
}

// --- listing and searching ---

// listIn is the shared query surface of the four repository lists. The sugar
// flags all end up in the one untyped filter bag the API takes, which is exactly
// the kind of detail a command line should absorb: a task, a library, a
// language, and a license are the same parameter upstream and four different
// concepts to a person.
type listIn struct {
	C         *Client  `kit:"inject"`
	Search    string   `kit:"flag" help:"full text search over ids and descriptions"`
	Author    string   `kit:"flag" help:"restrict to one owner"`
	Filter    []string `kit:"flag" help:"raw filter tags, ANDed together"`
	Task      []string `kit:"flag" help:"pipeline tag or task category, e.g. text-generation"`
	Library   []string `kit:"flag" help:"library, e.g. transformers"`
	Language  []string `kit:"flag" help:"language code, e.g. en"`
	License   []string `kit:"flag" help:"license, e.g. apache-2.0"`
	Size      []string `kit:"flag" help:"dataset size category, e.g. 1M<n<10M"`
	Benchmark []string `kit:"flag" help:"dataset benchmark tag"`
	SDK       []string `kit:"flag" help:"space SDK, e.g. gradio, streamlit, docker"`
	Sort      string   `kit:"flag" help:"downloads, likes, createdAt, lastModified, or trendingScore" enum:"downloads,likes,createdAt,lastModified,trendingScore"`
	Direction string   `kit:"flag" help:"asc or desc" enum:"asc,desc"`
	Full      bool     `kit:"flag" help:"ask for near-detail rows instead of stubs"`
	Limit     int      `kit:"flag,inherit"`
}

// options folds the sugar into the one parameter the API has.
func (in listIn) options() ListOptions {
	o := ListOptions{
		Search:    in.Search,
		Author:    in.Author,
		Sort:      in.Sort,
		Direction: in.Direction,
		Limit:     in.Limit,
		Expand:    in.Full,
	}
	o.Filter = append(o.Filter, in.Filter...)
	for _, group := range [][]string{in.Task, in.Library, in.Language, in.License, in.Size, in.Benchmark, in.SDK} {
		o.Filter = append(o.Filter, group...)
	}
	return o
}

func registerListOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "models", Group: "list", URIType: KindModel, List: true,
		Summary: "List models",
	}, listModels)

	kit.Handle(app, kit.OpMeta{
		Name: "datasets", Group: "list", URIType: KindDataset, List: true,
		Summary: "List datasets",
	}, listDatasets)

	kit.Handle(app, kit.OpMeta{
		Name: "spaces", Group: "list", URIType: KindSpace, List: true,
		Summary: "List spaces",
	}, listSpaces)

	kit.Handle(app, kit.OpMeta{
		Name: "kernels", Group: "list", URIType: KindKernel, List: true,
		Summary: "List compute kernels",
	}, listKernels)

	kit.Handle(app, kit.OpMeta{
		Name: "collections", Group: "list", URIType: KindCollection, List: true,
		Summary: "List collections, or find the ones containing a repo",
	}, listCollections)

	kit.Handle(app, kit.OpMeta{
		Name: "papers", Group: "list", URIType: KindPaper, List: true,
		Summary: "List papers, by search or by daily feed",
	}, listPapers)

	kit.Handle(app, kit.OpMeta{
		Name: "posts", Group: "list", URIType: KindPost, List: true,
		Summary: "List community posts",
	}, listPosts)

	kit.Handle(app, kit.OpMeta{
		Name: "blogs", Group: "list", URIType: KindBlog, List: true,
		Summary: "List blog entries from the RSS index",
	}, listBlogs)

	kit.Handle(app, kit.OpMeta{
		Name: "discussions", Group: "list", URIType: KindDiscussion, List: true,
		Summary: "List a repository's discussions and pull requests",
		Args:    []kit.Arg{{Name: "repo", Help: "repository ref"}},
	}, listDiscussions)

	kit.Handle(app, kit.OpMeta{
		Name: "search", Group: "list",
		Summary: "Search every entity kind at once",
		Args:    []kit.Arg{{Name: "query", Help: "what to look for"}},
	}, search)

	kit.Handle(app, kit.OpMeta{
		Name: "trending", Group: "list",
		Summary: "List what is trending right now",
	}, trending)

	kit.Handle(app, kit.OpMeta{
		Name: "tags", Group: "meta", URIType: KindTag, List: true,
		Summary: "List the controlled vocabulary",
		Args:    []kit.Arg{{Name: "type", Help: "one tag type, e.g. license or library", Optional: true}},
	}, listTags)

	kit.Handle(app, kit.OpMeta{
		Name: "tasks", Group: "meta", URIType: KindTask, List: true,
		Summary: "List the curated task pages",
	}, listTasks)

	kit.Handle(app, kit.OpMeta{
		Name: "sitemap", Group: "meta",
		Summary: "List every id the hub publishes in its sitemaps",
		Args:    []kit.Arg{{Name: "kind", Help: "models, datasets, spaces, or users", Optional: true}},
	}, listSitemap)

	kit.Handle(app, kit.OpMeta{
		Name: "dump", Group: "meta",
		Summary: "Fetch every record of one kind",
		Long: "dump walks the sitemap for ids and fetches a detail record for each one.\n" +
			"A full sweep is far past 100000 requests, so it refuses to start unbounded\n" +
			"without --yes. Pair it with --db to make the run resumable.",
		Args: []kit.Arg{{Name: "kind", Help: "model, dataset, space, or user"}},
	}, dump)
}

func listModels(ctx context.Context, in listIn, emit func(*Model) error) error {
	return in.C.Models(ctx, in.options(), emit)
}

func listDatasets(ctx context.Context, in listIn, emit func(*Dataset) error) error {
	return in.C.Datasets(ctx, in.options(), emit)
}

func listSpaces(ctx context.Context, in listIn, emit func(*Space) error) error {
	return in.C.Spaces(ctx, in.options(), emit)
}

func listKernels(ctx context.Context, in listIn, emit func(*Kernel) error) error {
	return in.C.Kernels(ctx, in.options(), emit)
}

type collectionListIn struct {
	C     *Client `kit:"inject"`
	Owner string  `kit:"flag" help:"restrict to one owner"`
	Item  string  `kit:"flag" help:"find the collections containing this item, e.g. models/google-bert/bert-base-uncased"`
	Sort  string  `kit:"flag" help:"lastModified, trending, or upvotes" enum:"lastModified,trending,upvotes"`
	Limit int     `kit:"flag,inherit"`
}

func listCollections(ctx context.Context, in collectionListIn, emit func(*Collection) error) error {
	return in.C.Collections(ctx, CollectionOptions{
		Owner: in.Owner,
		Item:  in.Item,
		Sort:  in.Sort,
		Limit: in.Limit,
	}, emit)
}

type paperListIn struct {
	C     *Client `kit:"inject"`
	Q     string  `kit:"flag" help:"search papers by title and abstract"`
	Date  string  `kit:"flag" help:"a daily feed date, YYYY-MM-DD"`
	Daily bool    `kit:"flag" help:"read the daily papers feed"`
	Sort  string  `kit:"flag" help:"publishedAt or trending" enum:"publishedAt,trending"`
	Limit int     `kit:"flag,inherit"`
}

func listPapers(ctx context.Context, in paperListIn, emit func(*Paper) error) error {
	if in.Daily || in.Date != "" {
		return in.C.DailyPapers(ctx, in.Date, in.Sort, in.Limit, emit)
	}
	if in.Q == "" {
		return errs.Usage("papers needs --q to search or --daily for the feed")
	}
	return in.C.SearchPapers(ctx, in.Q, in.Limit, emit)
}

type limitIn struct {
	C     *Client `kit:"inject"`
	Limit int     `kit:"flag,inherit"`
}

func listPosts(ctx context.Context, in limitIn, emit func(*Post) error) error {
	return in.C.Posts(ctx, in.Limit, emit)
}

func listBlogs(ctx context.Context, in limitIn, emit func(*BlogPost) error) error {
	return in.C.BlogFeed(ctx, in.Limit, emit)
}

type discussionListIn struct {
	C      *Client `kit:"inject"`
	Repo   string  `kit:"arg" help:"repository ref"`
	Status string  `kit:"flag" help:"open or closed" enum:"open,closed"`
	Type   string  `kit:"flag" help:"discussion or pull_request" enum:"discussion,pull_request"`
	Author string  `kit:"flag" help:"restrict to one author"`
	Limit  int     `kit:"flag,inherit"`
}

func listDiscussions(ctx context.Context, in discussionListIn, emit func(*Discussion) error) error {
	kind, id, err := ResolveRepo(in.Repo)
	if err != nil {
		return err
	}
	return in.C.Discussions(ctx, kind, id, DiscussionOptions{
		Status: in.Status,
		Type:   in.Type,
		Author: in.Author,
		Limit:  in.Limit,
	}, emit)
}

type searchIn struct {
	C     *Client  `kit:"inject"`
	Query string   `kit:"arg" help:"what to look for"`
	Type  []string `kit:"flag" help:"restrict to some kinds, e.g. model,dataset"`
	Limit int      `kit:"flag,inherit"`
}

func search(ctx context.Context, in searchIn, emit func(*Hit) error) error {
	_, err := in.C.Quicksearch(ctx, in.Query, in.Type, in.Limit, emit)
	return err
}

type trendingIn struct {
	C     *Client `kit:"inject"`
	Type  string  `kit:"flag" help:"model, dataset, or space" enum:"model,dataset,space"`
	Limit int     `kit:"flag,inherit"`
}

func trending(ctx context.Context, in trendingIn, emit func(*Hit) error) error {
	return in.C.Trending(ctx, in.Type, in.Limit, emit)
}

type tagsIn struct {
	C    *Client `kit:"inject"`
	Type string  `kit:"arg" help:"one tag type, e.g. license or library"`
}

func listTags(ctx context.Context, in tagsIn, emit func(*Tag) error) error {
	tax, err := in.C.Taxonomy(ctx)
	if err != nil {
		return err
	}
	all := tax.Tags()
	if in.Type == "" {
		return emitEach(all, emit)
	}
	for i := range all {
		if all[i].Type != in.Type {
			continue
		}
		if err := emit(&all[i]); err != nil {
			return err
		}
	}
	return nil
}

func listTasks(ctx context.Context, in clientIn, emit func(*Task) error) error {
	tasks, err := in.C.Tasks(ctx)
	if err != nil {
		return err
	}
	return emitEach(tasks, emit)
}

type sitemapIn struct {
	C     *Client `kit:"inject"`
	Kind  string  `kit:"arg" help:"models, datasets, spaces, or users"`
	Limit int     `kit:"flag,inherit"`
}

func listSitemap(ctx context.Context, in sitemapIn, emit func(*SitemapEntry) error) error {
	return in.C.Sitemap(ctx, in.Kind, in.Limit, emit)
}

type dumpIn struct {
	C       *Client `kit:"inject"`
	Kind    string  `kit:"arg" help:"model, dataset, space, or user"`
	IDsOnly bool    `kit:"flag" help:"emit sitemap entries instead of fetching each record"`
	Yes     bool    `kit:"flag" help:"confirm an unbounded sweep"`
	Jobs    int     `kit:"flag" help:"concurrent fetches (0 = the client default)"`
	Limit   int     `kit:"flag,inherit"`
}

func dump(ctx context.Context, in dumpIn, emit func(any) error) error {
	// People type the plural because the list commands are plural. Both mean
	// the same sweep.
	kind := strings.TrimSuffix(strings.ToLower(in.Kind), "s")
	return in.C.Dump(ctx, DumpOptions{
		Kind:    kind,
		Limit:   in.Limit,
		Workers: in.Jobs,
		Yes:     in.Yes,
		IDsOnly: in.IDsOnly,
	}, emit)
}

// --- relations ---

func registerRelationOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "likers", Group: "social",
		Summary: "List the users who liked a repository",
		Args:    []kit.Arg{{Name: "repo", Help: "repository ref"}},
	}, likers)

	kit.Handle(app, kit.OpMeta{
		Name: "likes", Group: "social",
		Summary: "List what a user has liked",
		Args:    []kit.Arg{{Name: "name", Help: "user or org name"}},
	}, likes)

	kit.Handle(app, kit.OpMeta{
		Name: "followers", Group: "social",
		Summary: "List a user's followers",
		Args:    []kit.Arg{{Name: "name", Help: "user name"}},
	}, followers)

	kit.Handle(app, kit.OpMeta{
		Name: "following", Group: "social",
		Summary: "List who a user follows",
		Args:    []kit.Arg{{Name: "name", Help: "user name"}},
	}, following)

	kit.Handle(app, kit.OpMeta{
		Name: "members", Group: "social",
		Summary: "List an organization's members",
		Args:    []kit.Arg{{Name: "name", Help: "org name"}},
	}, members)

	kit.Handle(app, kit.OpMeta{
		Name: "upvoters", Group: "social",
		Summary: "List who upvoted a paper or a collection",
		Long: "The hub has no endpoint for this, so the record comes from the rendered\n" +
			"page. It is the one relation that always costs an HTML fetch.",
		Args: []kit.Arg{{Name: "ref", Help: "paper or collection ref"}},
	}, upvoters)

	kit.Handle(app, kit.OpMeta{
		Name: "children", Group: "graph", URIType: KindModel,
		Summary: "List the models derived from a model",
		Long: "children runs the base_model filter once per relation (finetune, adapter,\n" +
			"quantized, merge) and merges the results, which is the only way to get a\n" +
			"model's descendants and is not obvious from the API docs.",
		Args: []kit.Arg{{Name: "model", Help: "model ref"}},
	}, children)

	kit.Handle(app, kit.OpMeta{
		Name: "parents", Group: "graph", URIType: KindModel,
		Summary: "List the models a model was derived from",
		Args:    []kit.Arg{{Name: "model", Help: "model ref"}},
	}, parents)

	kit.Handle(app, kit.OpMeta{
		Name: "citations", Group: "graph",
		Summary: "List the repositories tagged with a paper",
		Args:    []kit.Arg{{Name: "paper", Help: "arXiv id or paper URL"}},
	}, citations)
}

type repoLimitIn struct {
	C     *Client `kit:"inject"`
	Repo  string  `kit:"arg" help:"repository ref"`
	Limit int     `kit:"flag,inherit"`
}

func likers(ctx context.Context, in repoLimitIn, emit func(*UserRef) error) error {
	kind, id, err := ResolveRepo(in.Repo)
	if err != nil {
		return err
	}
	return in.C.Likers(ctx, kind, id, in.Limit, emit)
}

type nameLimitIn struct {
	C     *Client `kit:"inject"`
	Name  string  `kit:"arg" help:"user or org name"`
	Limit int     `kit:"flag,inherit"`
}

func likes(ctx context.Context, in nameLimitIn, emit func(*Like) error) error {
	name, err := ResolveRef(KindNamespace, in.Name)
	if err != nil {
		return err
	}
	return in.C.Likes(ctx, name, in.Limit, emit)
}

func followers(ctx context.Context, in nameLimitIn, emit func(*UserRef) error) error {
	name, err := ResolveRef(KindNamespace, in.Name)
	if err != nil {
		return err
	}
	return in.C.Followers(ctx, name, in.Limit, emit)
}

func following(ctx context.Context, in nameLimitIn, emit func(*UserRef) error) error {
	name, err := ResolveRef(KindNamespace, in.Name)
	if err != nil {
		return err
	}
	return in.C.Following(ctx, name, in.Limit, emit)
}

func members(ctx context.Context, in nameLimitIn, emit func(*UserRef) error) error {
	name, err := ResolveRef(KindOrg, in.Name)
	if err != nil {
		return err
	}
	return in.C.Members(ctx, name, in.Limit, emit)
}

func upvoters(ctx context.Context, in nameIn, emit func(*UserRef) error) error {
	kind, id, err := Classify(in.Name)
	if err != nil {
		return err
	}
	// Upvoters only exist on the page, so this one command turns the page plane
	// on for itself rather than returning nothing when the caller forgot --deep.
	ctx = WithDeep(ctx)
	switch kind {
	case KindPaper:
		p, err := in.C.DeepPaper(ctx, id)
		if err != nil {
			return err
		}
		return emitEach(p.Upvoters, emit)
	case KindCollection:
		col, err := in.C.DeepCollection(ctx, id)
		if err != nil {
			return err
		}
		return emitEach(col.Upvoters, emit)
	default:
		return errs.Usage("only papers and collections have upvoters, %q is a %s", in.Name, kind)
	}
}

type modelLimitIn struct {
	C     *Client `kit:"inject"`
	Model string  `kit:"arg" help:"model ref"`
	Limit int     `kit:"flag,inherit"`
}

func children(ctx context.Context, in modelLimitIn, emit func(*Model) error) error {
	id, err := ResolveRef(KindModel, in.Model)
	if err != nil {
		return err
	}
	return in.C.Children(ctx, id, in.Limit, emit)
}

func parents(ctx context.Context, in modelLimitIn, emit func(*Model) error) error {
	id, err := ResolveRef(KindModel, in.Model)
	if err != nil {
		return err
	}
	return in.C.Parents(ctx, id, emit)
}

type citationsIn struct {
	C     *Client  `kit:"inject"`
	Paper string   `kit:"arg" help:"arXiv id or paper URL"`
	Type  []string `kit:"flag" help:"restrict to some kinds, e.g. model,dataset"`
	Limit int      `kit:"flag,inherit"`
}

func citations(ctx context.Context, in citationsIn, emit func(any) error) error {
	id, err := ResolveRef(KindPaper, in.Paper)
	if err != nil {
		return err
	}
	return in.C.Citations(ctx, id, in.Type, in.Limit, emit)
}

// --- repository contents ---

func registerRepoOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "refs", Group: "repo", URIType: KindRef, List: true,
		Summary: "List a repository's branches, tags, and conversions",
		Args:    []kit.Arg{{Name: "repo", Help: "repository ref"}},
	}, refs)

	kit.Handle(app, kit.OpMeta{
		Name: "tree", Group: "repo", URIType: KindFile, List: true,
		Summary: "List the files in a repository",
		Args: []kit.Arg{
			{Name: "repo", Help: "repository ref"},
			{Name: "path", Help: "subdirectory", Optional: true},
		},
	}, tree)

	kit.Handle(app, kit.OpMeta{
		Name: "files", Group: "repo",
		Summary: "List every file in a repository, flat",
		Args:    []kit.Arg{{Name: "repo", Help: "repository ref"}},
	}, files)

	kit.Handle(app, kit.OpMeta{
		Name: "commits", Group: "repo", URIType: KindCommit, List: true,
		Summary: "List a repository's commits",
		Args:    []kit.Arg{{Name: "repo", Help: "repository ref"}},
	}, commits)

	kit.Handle(app, kit.OpMeta{
		Name: "card", Group: "repo", Single: true,
		Summary: "Read the parsed README front matter",
		Args:    []kit.Arg{{Name: "repo", Help: "repository ref"}},
	}, card)

	kit.Handle(app, kit.OpMeta{
		Name: "paths", Group: "repo",
		Summary: "Read file metadata and security scans for named paths",
		Args: []kit.Arg{
			{Name: "repo", Help: "repository ref"},
			{Name: "path", Help: "one or more paths", Variadic: true},
		},
	}, paths)

	kit.Handle(app, kit.OpMeta{
		Name: "page", Group: "repo", Single: true,
		Summary: "Read a rendered page as JSON, 1:1 with what the browser gets",
		Long: "page is deliberately low-level: it gives you the page's own hydration\n" +
			"payloads, JSON-LD, and meta tags, organised but not interpreted. When a\n" +
			"field appears on the site that hf does not model yet, this is how you get\n" +
			"it today and how the gap gets found.",
		Args: []kit.Arg{{Name: "ref", Help: "any hub reference or URL"}},
	}, page)
}

type treeIn struct {
	C         *Client `kit:"inject"`
	Repo      string  `kit:"arg" help:"repository ref"`
	Path      string  `kit:"arg" help:"subdirectory"`
	Rev       string  `kit:"flag" help:"branch, tag, or commit sha"`
	Recursive bool    `kit:"flag,short=r" help:"descend into subdirectories"`
	Commits   bool    `kit:"flag" help:"include the last commit and security scan per entry"`
	Limit     int     `kit:"flag,inherit"`
}

func refs(ctx context.Context, in bareRefIn, emit func(*Ref) error) error {
	kind, id, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	list, err := in.C.Refs(ctx, kind, id)
	if err != nil {
		return err
	}
	return emitEach(list, emit)
}

func tree(ctx context.Context, in treeIn, emit func(*TreeEntry) error) error {
	kind, id, err := ResolveRepo(in.Repo)
	if err != nil {
		return err
	}
	return in.C.Tree(ctx, kind, id, TreeOptions{
		Revision:  in.Rev,
		Path:      in.Path,
		Recursive: in.Recursive,
		Commits:   in.Commits,
		Limit:     in.Limit,
	}, emit)
}

func files(ctx context.Context, in repoLimitIn, emit func(*TreeEntry) error) error {
	kind, id, err := ResolveRepo(in.Repo)
	if err != nil {
		return err
	}
	return in.C.Tree(ctx, kind, id, TreeOptions{Recursive: true, Limit: in.Limit}, emit)
}

type commitsIn struct {
	C     *Client `kit:"inject"`
	Repo  string  `kit:"arg" help:"repository ref"`
	Rev   string  `kit:"flag" help:"branch, tag, or commit sha"`
	Limit int     `kit:"flag,inherit"`
}

func commits(ctx context.Context, in commitsIn, emit func(*Commit) error) error {
	kind, id, err := ResolveRepo(in.Repo)
	if err != nil {
		return err
	}
	return in.C.Commits(ctx, kind, id, in.Rev, in.Limit, emit)
}

func card(ctx context.Context, in refIn, emit func(*Card) error) error {
	kind, id, err := ResolveRepo(in.Ref)
	if err != nil {
		return err
	}
	readme, err := in.C.Readme(ctx, kind, id, in.Rev)
	if err != nil {
		return err
	}
	parsed, _, err := ParseReadme(readme)
	if err != nil {
		return err
	}
	if parsed == nil {
		return errs.NoResults("%s has no front matter", id)
	}
	return emit(parsed)
}

type pathsIn struct {
	C    *Client  `kit:"inject"`
	Repo string   `kit:"arg" help:"repository ref"`
	Path []string `kit:"arg,variadic" help:"one or more paths"`
	Rev  string   `kit:"flag" help:"branch, tag, or commit sha"`
}

func paths(ctx context.Context, in pathsIn, emit func(*TreeEntry) error) error {
	kind, id, err := ResolveRepo(in.Repo)
	if err != nil {
		return err
	}
	if len(in.Path) == 0 {
		return errs.Usage("paths needs at least one path")
	}
	list, err := in.C.PathsInfo(ctx, kind, id, in.Rev, in.Path)
	if err != nil {
		return err
	}
	return emitEach(list, emit)
}

func page(ctx context.Context, in bareRefIn, emit func(*Page) error) error {
	kind, id, err := Classify(in.Ref)
	if err != nil {
		return err
	}
	p, err := in.C.PageOf(ctx, kind, id)
	if err != nil {
		return err
	}
	return emit(p)
}

// --- dataset contents ---

// rowIn is the shared input of the viewer commands. Config and split are
// optional everywhere: when they are missing the first split of the first
// config is used, which is what a person means by `hf head squad`.
type rowIn struct {
	C       *Client `kit:"inject"`
	Dataset string  `kit:"arg" help:"dataset ref"`
	Config  string  `kit:"flag" help:"dataset config (default: the first one)"`
	Split   string  `kit:"flag" help:"split (default: train, or the first one)"`
	Offset  int64   `kit:"flag" help:"start at this row"`
	OrderBy string  `kit:"flag" help:"SQL-ish ORDER BY clause"`
	Limit   int     `kit:"flag,inherit"`
}

func (in rowIn) options() RowOptions {
	return RowOptions{
		Config:  in.Config,
		Split:   in.Split,
		Offset:  in.Offset,
		OrderBy: in.OrderBy,
		Limit:   in.Limit,
	}
}

// resolveSplit fills in a config and split when the caller did not name one.
// train wins when it exists, because it is what a dataset is mostly made of.
func resolveSplit(ctx context.Context, c *Client, dataset string, o RowOptions) (RowOptions, error) {
	if o.Config != "" && o.Split != "" {
		return o, nil
	}
	all, err := c.Splits(ctx, dataset)
	if err != nil {
		return o, err
	}
	var candidates []Split
	for _, s := range all {
		if o.Config != "" && s.Config != o.Config {
			continue
		}
		if o.Split != "" && s.Split != o.Split {
			continue
		}
		candidates = append(candidates, s)
	}
	if len(candidates) == 0 {
		return o, errs.NoResults("%s has no viewable split matching config %q split %q", dataset, o.Config, o.Split)
	}
	pick := candidates[0]
	for _, s := range candidates {
		if s.Config == pick.Config && s.Split == "train" {
			pick = s
			break
		}
	}
	o.Config, o.Split = pick.Config, pick.Split
	return o, nil
}

func registerDataOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "valid", Group: "data", Single: true,
		Summary: "Report which viewer capabilities a dataset has",
		Args:    []kit.Arg{{Name: "dataset", Help: "dataset ref"}},
	}, valid)

	kit.Handle(app, kit.OpMeta{
		Name: "splits", Group: "data", URIType: KindSplit, List: true,
		Summary: "List a dataset's configs and splits",
		Args:    []kit.Arg{{Name: "dataset", Help: "dataset ref"}},
	}, splits)

	kit.Handle(app, kit.OpMeta{
		Name: "size", Group: "data",
		Summary: "Report a dataset's row and byte counts",
		Args:    []kit.Arg{{Name: "dataset", Help: "dataset ref"}},
	}, size)

	kit.Handle(app, kit.OpMeta{
		Name: "rows", Group: "data",
		Summary: "Read a split's rows",
		Args:    []kit.Arg{{Name: "dataset", Help: "dataset ref"}},
	}, rows)

	kit.Handle(app, kit.OpMeta{
		Name: "head", Group: "data",
		Summary: "Read the first rows of a split",
		Args:    []kit.Arg{{Name: "dataset", Help: "dataset ref"}},
	}, head)

	kit.Handle(app, kit.OpMeta{
		Name: "schema", Group: "data",
		Summary: "Read a split's column types",
		Args:    []kit.Arg{{Name: "dataset", Help: "dataset ref"}},
	}, schema)

	kit.Handle(app, kit.OpMeta{
		Name: "stats", Group: "data",
		Summary: "Read per-column statistics for a split",
		Args:    []kit.Arg{{Name: "dataset", Help: "dataset ref"}},
	}, stats)

	kit.Handle(app, kit.OpMeta{
		Name: "dsearch", Group: "data",
		Summary: "Full text search within a split",
		Args: []kit.Arg{
			{Name: "dataset", Help: "dataset ref"},
			{Name: "query", Help: "what to look for"},
		},
	}, dsearch)

	kit.Handle(app, kit.OpMeta{
		Name: "filter", Group: "data",
		Summary: "Filter a split with a SQL-ish predicate",
		Args: []kit.Arg{
			{Name: "dataset", Help: "dataset ref"},
			{Name: "where", Help: `a predicate, e.g. "title"='Beyoncé'`},
		},
	}, filter)

	kit.Handle(app, kit.OpMeta{
		Name: "parquet", Group: "data",
		Summary: "List a dataset's converted parquet shards",
		Args:    []kit.Arg{{Name: "dataset", Help: "dataset ref"}},
	}, parquet)
}

type datasetIn struct {
	C       *Client `kit:"inject"`
	Dataset string  `kit:"arg" help:"dataset ref"`
}

func valid(ctx context.Context, in datasetIn, emit func(*Validity) error) error {
	id, err := ResolveRef(KindDataset, in.Dataset)
	if err != nil {
		return err
	}
	v, err := in.C.IsValid(ctx, id)
	if err != nil {
		return err
	}
	return emit(v)
}

func splits(ctx context.Context, in datasetIn, emit func(*Split) error) error {
	id, err := ResolveRef(KindDataset, in.Dataset)
	if err != nil {
		return err
	}
	list, err := in.C.Splits(ctx, id)
	if err != nil {
		return err
	}
	return emitEach(list, emit)
}

func size(ctx context.Context, in datasetIn, emit func(*Size) error) error {
	id, err := ResolveRef(KindDataset, in.Dataset)
	if err != nil {
		return err
	}
	list, err := in.C.Sizes(ctx, id)
	if err != nil {
		return err
	}
	return emitEach(list, emit)
}

func rows(ctx context.Context, in rowIn, emit func(*Row) error) error {
	id, o, err := datasetAndSplit(ctx, in)
	if err != nil {
		return err
	}
	return in.C.Rows(ctx, id, o, nil, emit)
}

func head(ctx context.Context, in rowIn, emit func(*Row) error) error {
	id, o, err := datasetAndSplit(ctx, in)
	if err != nil {
		return err
	}
	if o.Limit <= 0 {
		o.Limit = 100
	}
	return in.C.Rows(ctx, id, o, nil, emit)
}

func schema(ctx context.Context, in rowIn, emit func(*Feature) error) error {
	id, o, err := datasetAndSplit(ctx, in)
	if err != nil {
		return err
	}
	list, err := in.C.Features(ctx, id, o)
	if err != nil {
		return err
	}
	return emitEach(list, emit)
}

func stats(ctx context.Context, in rowIn, emit func(*ColumnStats) error) error {
	id, o, err := datasetAndSplit(ctx, in)
	if err != nil {
		return err
	}
	list, err := in.C.Statistics(ctx, id, o)
	if err != nil {
		return err
	}
	return emitEach(list, emit)
}

type dsearchIn struct {
	C       *Client `kit:"inject"`
	Dataset string  `kit:"arg" help:"dataset ref"`
	Query   string  `kit:"arg" help:"what to look for"`
	Config  string  `kit:"flag" help:"dataset config"`
	Split   string  `kit:"flag" help:"split"`
	Offset  int64   `kit:"flag" help:"start at this row"`
	Limit   int     `kit:"flag,inherit"`
}

func dsearch(ctx context.Context, in dsearchIn, emit func(*Row) error) error {
	id, err := ResolveRef(KindDataset, in.Dataset)
	if err != nil {
		return err
	}
	o, err := resolveSplit(ctx, in.C, id, RowOptions{
		Config: in.Config, Split: in.Split, Offset: in.Offset, Limit: in.Limit, Query: in.Query,
	})
	if err != nil {
		return err
	}
	return in.C.Rows(ctx, id, o, nil, emit)
}

type filterIn struct {
	C       *Client `kit:"inject"`
	Dataset string  `kit:"arg" help:"dataset ref"`
	Where   string  `kit:"arg" help:"a predicate, e.g. \"title\"='Beyoncé'"`
	Config  string  `kit:"flag" help:"dataset config"`
	Split   string  `kit:"flag" help:"split"`
	Offset  int64   `kit:"flag" help:"start at this row"`
	OrderBy string  `kit:"flag" help:"SQL-ish ORDER BY clause"`
	Limit   int     `kit:"flag,inherit"`
}

func filter(ctx context.Context, in filterIn, emit func(*Row) error) error {
	id, err := ResolveRef(KindDataset, in.Dataset)
	if err != nil {
		return err
	}
	o, err := resolveSplit(ctx, in.C, id, RowOptions{
		Config: in.Config, Split: in.Split, Offset: in.Offset,
		Limit: in.Limit, Where: in.Where, OrderBy: in.OrderBy,
	})
	if err != nil {
		return err
	}
	return in.C.Rows(ctx, id, o, nil, emit)
}

func parquet(ctx context.Context, in datasetIn, emit func(*ParquetShard) error) error {
	id, err := ResolveRef(KindDataset, in.Dataset)
	if err != nil {
		return err
	}
	list, err := in.C.Parquet(ctx, id)
	if err != nil {
		return err
	}
	return emitEach(list, emit)
}

// datasetAndSplit resolves the dataset ref and fills in a default split, which
// every viewer command needs and none of them should repeat.
func datasetAndSplit(ctx context.Context, in rowIn) (string, RowOptions, error) {
	id, err := ResolveRef(KindDataset, in.Dataset)
	if err != nil {
		return "", RowOptions{}, err
	}
	o, err := resolveSplit(ctx, in.C, id, in.options())
	return id, o, err
}

// --- the graph plane ---

func registerGraphOps(app *kit.App) {
	kit.Handle(app, kit.OpMeta{
		Name: "graph", Group: "graph",
		Summary: "Emit the node and edges for one entity",
		Args:    []kit.Arg{{Name: "ref", Help: "any hub reference"}},
	}, graph)

	kit.Handle(app, kit.OpMeta{
		Name: "edges", Group: "graph",
		Summary: "Emit only the edges for one entity",
		Args:    []kit.Arg{{Name: "ref", Help: "any hub reference"}},
	}, edges)

	kit.Handle(app, kit.OpMeta{
		Name: "crawl", Group: "graph",
		Summary: "Walk the graph breadth-first from a seed",
		Long: "crawl follows only the predicates named by --follow, which defaults to the\n" +
			"structural ones. Nodes and edges stream out as they are discovered, and the\n" +
			"walk stops cleanly at --max-nodes or --max-requests rather than failing.",
		Args: []kit.Arg{{Name: "ref", Help: "seed reference"}},
	}, crawl)

	kit.Handle(app, kit.OpMeta{
		Name: "uri", Group: "graph", Single: true,
		Summary: "Resolve any reference to its canonical hf:// URI",
		Long:    "uri makes no request, so it is instant and works offline.",
		Args:    []kit.Arg{{Name: "input", Help: "an id, a URL, or an hf:// URI"}},
	}, uriOf)

	kit.Handle(app, kit.OpMeta{
		Name: "url", Group: "graph", Single: true,
		Summary: "Resolve any reference to its https location",
		Long:    "url makes no request, so it is instant and works offline.",
		Args:    []kit.Arg{{Name: "input", Help: "an id, a URL, or an hf:// URI"}},
	}, urlOf)
}

func graph(ctx context.Context, in bareRefIn, emit func(any) error) error {
	_, _, g, err := in.C.GraphOfRef(ctx, in.Ref)
	if err != nil {
		return err
	}
	for i := range g.Nodes {
		if err := emit(&g.Nodes[i]); err != nil {
			return err
		}
	}
	for i := range g.Edges {
		if err := emit(&g.Edges[i]); err != nil {
			return err
		}
	}
	return nil
}

func edges(ctx context.Context, in bareRefIn, emit func(*Edge) error) error {
	_, _, g, err := in.C.GraphOfRef(ctx, in.Ref)
	if err != nil {
		return err
	}
	return emitEach(g.Edges, emit)
}

type crawlIn struct {
	C         *Client  `kit:"inject"`
	Ref       string   `kit:"arg" help:"seed reference"`
	Depth     int      `kit:"flag" help:"how many edges out to walk" default:"1"`
	Follow    []string `kit:"flag" help:"predicates to follow (default: the structural ones)"`
	MaxNodes  int      `kit:"flag" help:"stop after this many nodes" default:"10000"`
	MaxReqs   int      `kit:"flag,name=max-requests" help:"stop after this many requests" default:"5000"`
	NodesOnly bool     `kit:"flag" help:"emit nodes only"`
	EdgesOnly bool     `kit:"flag" help:"emit edges only"`
}

func crawl(ctx context.Context, in crawlIn, emit func(any) error) error {
	kind, id, err := Classify(in.Ref)
	if err != nil {
		return err
	}
	return in.C.Crawl(ctx, URI(kind, id), CrawlOptions{
		Depth:     in.Depth,
		Follow:    in.Follow,
		MaxNodes:  in.MaxNodes,
		MaxReqs:   in.MaxReqs,
		NodesOnly: in.NodesOnly,
		EdgesOnly: in.EdgesOnly,
	}, CrawlSink{
		Node: func(n *Node) error { return emit(n) },
		Edge: func(e *Edge) error { return emit(e) },
	})
}

// Address is what the pure-function pair answers: one input, resolved. Both
// commands emit it, because the difference between them is which field of the
// same answer you wanted.
type Address struct {
	Meta

	Input string `json:"input"`
	ID    string `json:"id"`
}

type inputIn struct {
	C     *Client `kit:"inject"`
	Input string  `kit:"arg" help:"an id, a URL, or an hf:// URI"`
}

func resolveAddress(input string) (*Address, error) {
	kind, id, err := Classify(input)
	if err != nil {
		return nil, err
	}
	a := &Address{Input: input, ID: id}
	a.setMeta(kind, id)
	if a.URL == "" {
		return nil, errs.Unsupported("hf cannot locate a %s", kind)
	}
	return a, nil
}

func uriOf(_ context.Context, in inputIn, emit func(*Address) error) error {
	a, err := resolveAddress(in.Input)
	if err != nil {
		return err
	}
	return emit(a)
}

func urlOf(_ context.Context, in inputIn, emit func(*Address) error) error {
	a, err := resolveAddress(in.Input)
	if err != nil {
		return err
	}
	return emit(a)
}
