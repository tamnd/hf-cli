package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
	"github.com/tamnd/hf-cli/hf"
)

// export.go writes a whole graph to one file. Everything it does can be had by
// redirecting hf crawl or hf rdf, and it exists because the thing people
// actually want at the end of a walk is a single file they can load somewhere
// else, named once rather than assembled from two commands and a shell operator.

type exportCmd struct {
	format   string
	out      string
	depth    int
	follow   []string
	maxNodes int
}

func newExportCmd() kit.Command {
	c := &exportCmd{}
	return kit.Command{
		Use:   "export <ref>",
		Short: "Write a whole graph to one file",
		Long: "export walks from the seed and writes the result in one go. The graph formats\n" +
			"are jsonl (one node or edge per line), json (a single object with nodes and\n" +
			"edges), and the four RDF serialisations nt, ttl, jsonld, and nq.\n\n" +
			"Without --out it writes to stdout, which makes it a drop-in for a pipeline.",
		Group: "graph",
		Args:  kit.ExactArgs(1),
		Flags: c.flags,
		Run:   c.run,
	}
}

func (c *exportCmd) flags(f *kit.FlagSet) {
	f.StringVar(&c.format, "format", "jsonl", "jsonl, json, nt, ttl, jsonld, or nq")
	f.StringVarP(&c.out, "out", "O", "", "write here instead of stdout")
	f.IntVar(&c.depth, "depth", 1, "walk this many edges out")
	f.StringSliceVar(&c.follow, "follow", nil, "predicates to follow (default: the structural ones)")
	f.IntVar(&c.maxNodes, "max-nodes", 0, "stop after this many nodes")
}

func (c *exportCmd) run(ctx context.Context, args []string) error {
	cl, err := clientFrom(ctx)
	if err != nil {
		return err
	}
	kind, id, g, err := buildGraph(ctx, cl, args[0], c.depth, c.follow, c.maxNodes)
	if err != nil {
		return err
	}
	hf.SortEdges(g.Edges)

	out := io.Writer(os.Stdout)
	if c.out != "" {
		f, err := os.Create(c.out)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		out = f
	}
	w := bufio.NewWriter(out)
	defer func() { _ = w.Flush() }()

	switch c.format {
	case "jsonl":
		return writeJSONL(w, g)
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(g)
	case hf.FormatNT, hf.FormatTurtle, hf.FormatJSONLD, hf.FormatNQuads:
		graph, _ := hf.Locate(kind, id)
		return hf.WriteRDF(w, g, hf.RDFOptions{Format: c.format, Graph: graph})
	default:
		return errs.Usage("unknown --format %q", c.format)
	}
}

// writeJSONL puts the nodes first and the edges after, so a reader that builds
// an index in one pass never sees an edge before both of its ends.
func writeJSONL(w io.Writer, g *hf.Graph) error {
	enc := json.NewEncoder(w)
	for i := range g.Nodes {
		if err := enc.Encode(&g.Nodes[i]); err != nil {
			return err
		}
	}
	for i := range g.Edges {
		if err := enc.Encode(&g.Edges[i]); err != nil {
			return err
		}
	}
	return nil
}
