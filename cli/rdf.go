package cli

import (
	"bufio"
	"context"
	"os"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/hf-cli/hf"
)

// rdf.go holds the linked-data output commands. They are escape hatches for the
// same reason cat is: N-Triples and Turtle are not records, they are a
// serialisation with its own rules, and passing them through the record
// renderer would produce something that is neither.

type rdfCmd struct {
	format        string
	graph         string
	withCroissant bool
	depth         int
	follow        []string
	maxNodes      int
}

func newRDFCmd() kit.Command {
	c := &rdfCmd{}
	return kit.Command{
		Use:   "rdf <ref>",
		Short: "Write an entity as RDF triples",
		Long: "rdf serialises one entity and its edges. N-Triples is the default because it\n" +
			"streams line by line, so a deep walk never needs the whole graph in memory;\n" +
			"Turtle and JSON-LD do need it and say so by being slower on large graphs.\n\n" +
			"With --depth it walks first and serialises the whole result, which is how you\n" +
			"get a loadable dataset rather than a single subject.",
		Group: "graph",
		Args:  kit.ExactArgs(1),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *rdfCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.format, "format", hf.FormatNT, "nt, ttl, jsonld, or nq")
	f.StringVar(&c.graph, "graph", "", "the named graph for nq output (default: the entity URL)")
	f.BoolVar(&c.withCroissant, "with-croissant", false, "fold the hub's Croissant document into jsonld output")
	f.IntVar(&c.depth, "depth", 0, "walk this many edges out before serialising")
	f.StringSliceVar(&c.follow, "follow", nil, "predicates to follow when walking")
	f.IntVar(&c.maxNodes, "max-nodes", 0, "stop a walk after this many nodes")
}

func (c *rdfCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	var kind, id string
	var g *hf.Graph
	if c.withCroissant {
		// Only a dataset has one, and a bare name classifies as a model, so the
		// flag settles the kind rather than arguing with it.
		if id, err = hf.ResolveRef(hf.KindDataset, args[0]); err != nil {
			return err
		}
		kind = hf.KindDataset
		if g, err = buildGraphOf(ctx, cl, kind, id, c.depth, c.follow, c.maxNodes); err != nil {
			return err
		}
	} else if kind, id, g, err = buildGraph(ctx, cl, args[0], c.depth, c.follow, c.maxNodes); err != nil {
		return err
	}
	hf.SortEdges(g.Edges)

	w := bufio.NewWriter(os.Stdout)
	defer func() { _ = w.Flush() }()

	if c.withCroissant {
		doc, err := cl.Croissant(ctx, id)
		if err != nil {
			return err
		}
		return hf.MergeCroissant(doc, g, w)
	}

	graph := c.graph
	if graph == "" {
		graph, _ = hf.Locate(kind, id)
	}
	return hf.WriteRDF(w, g, hf.RDFOptions{Format: c.format, Graph: graph})
}

// buildGraph resolves a reference and returns the one entity or the whole walk,
// depending on depth. Both end up as a Graph, so the serialisers never need to
// know which it was. It holds the result in memory, which is the price of the
// formats that cannot stream; hf crawl is the streaming answer for a walk too
// big for this one.
func buildGraph(ctx context.Context, cl *hf.Client, ref string, depth int, follow []string, maxNodes int) (string, string, *hf.Graph, error) {
	kind, id, g, err := cl.GraphOfRef(ctx, ref)
	if err != nil {
		return "", "", nil, err
	}
	if depth <= 0 {
		return kind, id, g, nil
	}
	g, err = buildGraphOf(ctx, cl, kind, id, depth, follow, maxNodes)
	if err != nil {
		return "", "", nil, err
	}
	return kind, id, g, nil
}

// buildGraphOf is buildGraph for a caller that already knows the kind.
func buildGraphOf(ctx context.Context, cl *hf.Client, kind, id string, depth int, follow []string, maxNodes int) (*hf.Graph, error) {
	if depth <= 0 {
		return cl.GraphOf(ctx, kind, id)
	}
	g := &hf.Graph{}
	err := cl.Crawl(ctx, hf.URI(kind, id), hf.CrawlOptions{
		Depth:    depth,
		Follow:   follow,
		MaxNodes: maxNodes,
	}, hf.CrawlSink{
		Node: func(n *hf.Node) error { g.Nodes = append(g.Nodes, *n); return nil },
		Edge: func(e *hf.Edge) error { g.Edges = append(g.Edges, *e); return nil },
	})
	if err != nil {
		return nil, err
	}
	return g, nil
}

type croissantCmd struct{}

func newCroissantCmd() kit.Command {
	c := &croissantCmd{}
	return kit.Command{
		Use:   "croissant <dataset>",
		Short: "Write a dataset's Croissant metadata",
		Long: "croissant passes the hub's own document straight through. It is already a\n" +
			"correct standards-body serialisation, and reimplementing one would be\n" +
			"strictly worse than not touching it.",
		Group: "data",
		Args:  kit.ExactArgs(1),
		Run:   c.run,
	}
}

func (c *croissantCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	id, err := hf.ResolveRef(hf.KindDataset, args[0])
	if err != nil {
		return err
	}
	doc, err := cl.Croissant(ctx, id)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(os.Stdout)
	defer func() { _ = w.Flush() }()
	if _, err := w.Write(doc); err != nil {
		return err
	}
	_, err = w.WriteString("\n")
	return err
}
