package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tamnd/hf-cli/hf"
	"github.com/tamnd/hf-cli/hftest"
)

// cli_test.go covers the commands that write bytes rather than records. They
// are the only part of the surface the hf package's own tests cannot reach, and
// they share that package's fixtures: what is being tested here is the
// serialisation, not the fetch, so recording a second copy of the same
// exchanges would only mean two things to keep in step.

const fixtureDir = "../hf/testdata/live"

func replayClient(t *testing.T) *hf.Client {
	t.Helper()
	rp, err := hftest.NewReplayer(fixtureDir)
	if err != nil {
		t.Fatalf("%v\nrecord the fixtures first: make fixtures", err)
	}
	c := hf.NewClient()
	c.HTTP = &http.Client{Transport: rp}
	c.Token = ""
	c.Rate = 0
	c.Retries = 0
	c.NoCache = true
	c.Workers = 1
	return c
}

// TestRDF serialises one model and one dataset in every format. The golden is
// the predicate vocabulary rather than the triples themselves, because the
// object of a triple is a download count that moves and the predicate set is
// the part a consumer writes queries against.
func TestRDF(t *testing.T) {
	cl := replayClient(t)
	cases := []struct {
		name   string
		ref    string
		format string
	}{
		{"model-nt", "google-bert/bert-base-uncased", hf.FormatNT},
		{"model-ttl", "google-bert/bert-base-uncased", hf.FormatTurtle},
		{"model-jsonld", "google-bert/bert-base-uncased", hf.FormatJSONLD},
		{"model-nq", "google-bert/bert-base-uncased", hf.FormatNQuads},
		{"dataset-nt", "rajpurkar/squad", hf.FormatNT},
		{"dataset-jsonld", "rajpurkar/squad", hf.FormatJSONLD},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			kind, id, g, err := buildGraph(ctx, cl, tc.ref, 0, nil, 0)
			if err != nil {
				t.Fatalf("build graph: %v", err)
			}
			hf.SortEdges(g.Edges)
			graph, _ := hf.Locate(kind, id)

			var buf bytes.Buffer
			if err := hf.WriteRDF(&buf, g, hf.RDFOptions{Format: tc.format, Graph: graph}); err != nil {
				t.Fatalf("write %s: %v", tc.format, err)
			}
			if buf.Len() == 0 {
				t.Fatal("wrote nothing")
			}
			if tc.format == hf.FormatJSONLD {
				var v any
				if err := json.Unmarshal(buf.Bytes(), &v); err != nil {
					t.Fatalf("jsonld is not valid JSON: %v", err)
				}
			}
			checkGolden(t, "rdf-"+tc.name, vocabulary(buf.String()))
		})
	}
}

// TestCroissant checks that the hub's own dataset document survives the fold
// into JSON-LD. It is somebody else's standards-body serialisation, so the test
// that matters is that the keys it came with are still there afterwards.
func TestCroissant(t *testing.T) {
	cl := replayClient(t)
	ctx := context.Background()
	doc, err := cl.Croissant(ctx, "rajpurkar/squad")
	if err != nil {
		t.Fatal(err)
	}
	g, err := buildGraphOf(ctx, cl, hf.KindDataset, "rajpurkar/squad", 0, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := hf.MergeCroissant(doc, g, &buf); err != nil {
		t.Fatal(err)
	}
	var merged map[string]any
	if err := json.Unmarshal(buf.Bytes(), &merged); err != nil {
		t.Fatalf("merged croissant is not valid JSON: %v", err)
	}
	var original map[string]any
	if err := json.Unmarshal(doc, &original); err != nil {
		t.Fatal(err)
	}
	for k := range original {
		if _, ok := merged[k]; !ok {
			t.Errorf("the merge dropped %q, which the hub's own document had", k)
		}
	}
	checkGolden(t, "croissant", keysOf(merged))
}

// TestExport covers the two whole-graph formats.
func TestExport(t *testing.T) {
	cl := replayClient(t)
	_, _, g, err := buildGraph(context.Background(), cl, "google-bert/bert-base-uncased", 0, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	hf.SortEdges(g.Edges)

	var buf bytes.Buffer
	if err := writeJSONL(&buf, g); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != len(g.Nodes)+len(g.Edges) {
		t.Fatalf("jsonl has %d lines for %d nodes and %d edges", len(lines), len(g.Nodes), len(g.Edges))
	}
	// Nodes come first so a reader that indexes in one pass never meets an edge
	// before both of its ends.
	for i, line := range lines {
		var row struct {
			Predicate string `json:"predicate"`
		}
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("line %d is not JSON: %v", i+1, err)
		}
		if i < len(g.Nodes) && row.Predicate != "" {
			t.Fatalf("line %d is an edge but should be a node", i+1)
		}
		if i >= len(g.Nodes) && row.Predicate == "" {
			t.Fatalf("line %d is a node but should be an edge", i+1)
		}
	}
}

// TestGraphResolvesBareNames pins the behaviour that a bare owner/name is a
// guess. Most of the hub is models, so that is the guess, and it has to give
// way when the thing turns out to be a dataset.
func TestGraphResolvesBareNames(t *testing.T) {
	cl := replayClient(t)
	cases := map[string]string{
		"google-bert/bert-base-uncased": hf.KindModel,
		"rajpurkar/squad":               hf.KindDataset,
		"datasets/rajpurkar/squad":      hf.KindDataset,
	}
	for ref, want := range cases {
		kind, _, _, err := buildGraph(context.Background(), cl, ref, 0, nil, 0)
		if err != nil {
			t.Errorf("%s: %v", ref, err)
			continue
		}
		if kind != want {
			t.Errorf("%s resolved to %s, want %s", ref, kind, want)
		}
	}
}

// prefixes are the namespaces the exports declare. Turtle and JSON-LD write
// their terms short, so a golden that only looked for full URIs would record
// the context and nothing else, and a predicate could go missing without the
// file changing at all.
var prefixes = []string{"hf:", "schema:", "dct:", "rdf:", "rdfs:", "xsd:"}

// vocabulary lists the distinct terms a serialisation used. Every format spells
// them differently, so this looks for anything that reads like a term rather
// than trying to parse four grammars.
func vocabulary(out string) []string {
	seen := map[string]bool{}
	for _, field := range strings.FieldsFunc(out, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == '"' || r == ',' || r == '[' || r == ']' || r == '{' || r == '}'
	}) {
		term := strings.Trim(field, "<>;.")
		if isTerm(term) {
			seen[term] = true
		}
	}
	return sorted(seen)
}

func isTerm(term string) bool {
	if strings.HasPrefix(term, "http://") || strings.HasPrefix(term, "https://") {
		return strings.Contains(term, "schema.org") || strings.Contains(term, "purl.org") ||
			strings.Contains(term, "w3.org") || strings.Contains(term, "/ns#")
	}
	for _, p := range prefixes {
		if strings.HasPrefix(term, p) && len(term) > len(p) {
			return true
		}
	}
	return false
}

func keysOf(m map[string]any) []string {
	seen := map[string]bool{}
	for k := range m {
		seen[k] = true
	}
	return sorted(seen)
}

func sorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func checkGolden(t *testing.T, name string, got []string) {
	t.Helper()
	path := filepath.Join("testdata", name+".txt")
	body := strings.Join(got, "\n") + "\n"
	if os.Getenv("HF_UPDATE") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nwrite the goldens first: HF_UPDATE=1 go test ./cli", err)
	}
	if string(want) != body {
		t.Errorf("golden %s does not match:\nwant:\n%s\ngot:\n%s", path, want, body)
	}
}
