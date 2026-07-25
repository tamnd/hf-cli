package hf

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/tamnd/any-cli/kit/errs"
)

// crawl.go walks the graph. There is no cleverness here and there should not
// be: a breadth-first frontier with a visited set and a budget is the whole
// algorithm, and the interesting decisions are all about what not to follow.

// CrawlOptions bounds a walk. The budgets are the important part of this
// struct: a tool that can accidentally send two million requests to someone
// else's servers should be hard to point that way by accident.
type CrawlOptions struct {
	Depth    int
	Follow   []string
	MaxNodes int
	MaxReqs  int

	NodesOnly bool
	EdgesOnly bool
}

// CrawlSink receives what the walk discovers. Emission is streaming: a crawl of
// a large org must never need the whole graph in memory.
type CrawlSink struct {
	Node func(*Node) error
	Edge func(*Edge) error
}

// defaults fills the budgets from the spec and the structural follow set. Social
// predicates fan out hard, so they stay opt-in.
func (o *CrawlOptions) defaults() {
	if o.Depth <= 0 {
		o.Depth = 1
	}
	if o.MaxNodes <= 0 {
		o.MaxNodes = 10000
	}
	if o.MaxReqs <= 0 {
		o.MaxReqs = 5000
	}
	if len(o.Follow) == 0 {
		o.Follow = StructuralPredicates
	}
}

// followSet accepts both the bare name and the prefixed predicate, so
// --follow ownedBy and --follow hf:ownedBy mean the same thing.
func followSet(follow []string) map[string]bool {
	out := map[string]bool{}
	for _, f := range follow {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		out[strings.TrimPrefix(f, "hf:")] = true
	}
	return out
}

// Crawl walks outward from a seed URI. Nodes and edges come out as they are
// discovered, and the walk stops cleanly at either budget, reporting what it had
// rather than failing.
func (c *Client) Crawl(ctx context.Context, seed string, o CrawlOptions, sink CrawlSink) error {
	o.defaults()
	allow := followSet(o.Follow)

	type item struct {
		uri   string
		depth int
	}
	visited := map[string]bool{seed: true}
	frontier := []item{{seed, 0}}
	nodes, reqs := 0, 0

	for len(frontier) > 0 {
		cur := frontier[0]
		frontier = frontier[1:]

		if reqs >= o.MaxReqs || nodes >= o.MaxNodes {
			return nil
		}
		kind, id, ok := SplitURI(cur.uri)
		if !ok {
			continue
		}
		// Following a tag back to its repos is a filtered list of millions, not
		// a link. The crawler says so rather than trying.
		if kind == KindTag {
			continue
		}
		rec, err := c.Fetch(ctx, kind, id)
		reqs++
		if err != nil {
			// A single unreachable node must not end a walk that has already
			// produced useful output. A gated repo mid-crawl is normal.
			if IsNotFound(err) || IsNeedAuth(err) {
				continue
			}
			return err
		}

		tax, _ := c.Taxonomy(ctx)
		node, edges := Extract(rec, tax)
		if node.URI == "" {
			continue
		}
		nodes++
		if !o.EdgesOnly && sink.Node != nil {
			n := node
			if err := sink.Node(&n); err != nil {
				return err
			}
		}
		for i := range edges {
			if !o.NodesOnly && sink.Edge != nil {
				e := edges[i]
				if err := sink.Edge(&e); err != nil {
					return err
				}
			}
		}
		if cur.depth >= o.Depth {
			continue
		}
		for _, e := range edges {
			if e.Literal || strings.HasPrefix(e.Object, "_:") {
				continue
			}
			if !allow[strings.TrimPrefix(e.Predicate, "hf:")] {
				continue
			}
			// Cycles are normal on this graph: a uses and usedBy pair is one by
			// construction, and the visited set is the only defence needed.
			if visited[e.Object] {
				continue
			}
			visited[e.Object] = true
			frontier = append(frontier, item{e.Object, cur.depth + 1})
		}
	}
	return nil
}

// Fetch returns the record behind a kind and id. It is the crawler's
// dereferencer and the implementation of hf get, so a URI from any part of the
// tool resolves the same way. It goes through the Deep* methods, which are the
// plain fetch when --deep is off, so one switch serves both modes.
func (c *Client) Fetch(ctx context.Context, kind, id string) (any, error) {
	switch kind {
	case KindModel:
		return c.DeepModel(ctx, id, "")
	case KindDataset:
		return c.DeepDataset(ctx, id, "")
	case KindSpace:
		return c.DeepSpace(ctx, id, "")
	case KindKernel:
		return c.Kernel(ctx, id, "")
	case KindUser:
		return c.DeepUser(ctx, id)
	case KindOrg:
		return c.DeepOrg(ctx, id)
	case KindNamespace:
		nsKind, err := c.NamespaceKind(ctx, id)
		if err != nil {
			return nil, err
		}
		return c.Fetch(ctx, nsKind, id)
	case KindCollection:
		return c.DeepCollection(ctx, id)
	case KindPaper:
		return c.DeepPaper(ctx, id)
	case KindPost:
		return c.Post(ctx, id)
	case KindBlog:
		return c.DeepBlog(ctx, id)
	case KindTask:
		return c.Task(ctx, id)
	case KindDiscussion:
		repoKind, repo, num, ok := splitDiscussionID(id)
		if !ok {
			return nil, errs.Usage("bad discussion id %q", id)
		}
		n, err := strconv.Atoi(num)
		if err != nil {
			return nil, errs.Usage("bad discussion number %q", num)
		}
		return c.DeepDiscussion(ctx, repoKind, repo, n)
	default:
		return nil, errs.Unsupported("cannot fetch %s records", kind)
	}
}

// FetchRef reads whatever a reference points at and reports which kind it
// turned out to be. Classify guesses model for a bare owner/name because most
// of the hub is models, and the 404 is what says when the guess was wrong, so
// this retries as a dataset, a space, and then a kernel. The guess is right
// nearly always, which is why the fallback runs on failure rather than probing
// every kind up front.
func (c *Client) FetchRef(ctx context.Context, input string) (string, string, any, error) {
	kind, id, err := Classify(input)
	if err != nil {
		return "", "", nil, err
	}
	rec, err := c.Fetch(ctx, kind, id)
	if err == nil || !guessedModel(input, kind) || errs.KindOf(err) != errs.KindNotFound {
		return kind, id, rec, err
	}
	for _, alt := range []string{KindDataset, KindSpace, KindKernel} {
		if other, aerr := c.Fetch(ctx, alt, id); aerr == nil {
			return alt, id, other, nil
		}
	}
	return kind, id, nil, err
}

// guessedModel reports whether the model kind came from Classify's default
// rather than from the reference itself. A URL, a URI, and a prefixed id all
// name their kind; a bare owner/name does not.
func guessedModel(input, kind string) bool {
	return kind == KindModel && !strings.Contains(input, "://") && !strings.Contains(input, "/models/")
}

// GraphOf builds the node and edges for one entity, taxonomy included. This is
// what hf graph and hf rdf both call, so the two never disagree.
func (c *Client) GraphOf(ctx context.Context, kind, id string) (*Graph, error) {
	rec, err := c.Fetch(ctx, kind, id)
	if err != nil {
		return nil, err
	}
	return c.graphFrom(ctx, rec), nil
}

// GraphOfRef is GraphOf for a reference that has not been classified yet, so a
// bare dataset name reaches the serialisers instead of a not-found for a model
// nobody asked about.
func (c *Client) GraphOfRef(ctx context.Context, input string) (string, string, *Graph, error) {
	kind, id, rec, err := c.FetchRef(ctx, input)
	if err != nil {
		return "", "", nil, err
	}
	return kind, id, c.graphFrom(ctx, rec), nil
}

func (c *Client) graphFrom(ctx context.Context, rec any) *Graph {
	tax, _ := c.Taxonomy(ctx)
	g := &Graph{}
	g.Add(rec, tax)
	return g
}

// Children finds the models derived from one model. It is the filter query run
// once per relation and merged, which is the only way to get a model's
// descendants and is not obvious from the API docs.
func (c *Client) Children(ctx context.Context, id string, limit int, emit func(*Model) error) error {
	seen := map[string]bool{}
	var mu sync.Mutex
	sent := 0
	for _, rel := range []string{RelFinetune, RelAdapter, RelQuantized, RelMerge} {
		if limit > 0 && sent >= limit {
			return nil
		}
		want := 0
		if limit > 0 {
			want = limit - sent
		}
		err := c.Models(ctx, ListOptions{
			Filter: []string{TypeBase + ":" + rel + ":" + id},
			Limit:  want,
		}, func(m *Model) error {
			mu.Lock()
			defer mu.Unlock()
			if seen[m.ID] {
				return nil
			}
			seen[m.ID] = true
			sent++
			return emit(m)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// Parents is the other direction, and it is a read of the record rather than a
// query, because a model names its own bases.
func (c *Client) Parents(ctx context.Context, id string, emit func(*Model) error) error {
	m, err := c.Model(ctx, id, "")
	if err != nil {
		return err
	}
	refs := m.BaseModels
	if len(refs) == 0 && m.CardData != nil {
		refs = m.CardData.BaseModelRefs()
	}
	if len(refs) == 0 {
		return nil
	}
	for _, ref := range refs {
		parent, err := c.Model(ctx, ref.ID, "")
		if err != nil {
			if IsNotFound(err) {
				continue
			}
			return err
		}
		if err := emit(parent); err != nil {
			return err
		}
	}
	return nil
}

// Citations lists the repos tagged with a paper. The arxiv tag is the edge from
// a repo to the literature, and this is that edge read backwards.
func (c *Client) Citations(ctx context.Context, paperID string, kinds []string, limit int, emit func(any) error) error {
	filter := TypeArxiv + ":" + trimVersion(paperID)
	if len(kinds) == 0 {
		kinds = []string{KindModel, KindDataset, KindSpace}
	}
	sent := 0
	for _, kind := range kinds {
		if limit > 0 && sent >= limit {
			return nil
		}
		want := 0
		if limit > 0 {
			want = limit - sent
		}
		o := ListOptions{Filter: []string{filter}, Limit: want}
		var err error
		switch kind {
		case KindModel:
			err = c.Models(ctx, o, func(m *Model) error { sent++; return emit(m) })
		case KindDataset:
			err = c.Datasets(ctx, o, func(d *Dataset) error { sent++; return emit(d) })
		case KindSpace:
			err = c.Spaces(ctx, o, func(s *Space) error { sent++; return emit(s) })
		}
		if err != nil {
			return err
		}
	}
	return nil
}
