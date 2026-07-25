package hf

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// rdf.go serialises the graph. The graph plane is already triples, so export is
// a serialisation and not a transformation. The one real decision here is the
// schema.org alignment: a consumer that has never heard of hf:derivesFrom still
// understands schema:isBasedOn, so the standard predicate is emitted alongside
// the hub-specific one rather than instead of it.

// The namespaces. hfr is the resource namespace, and hf:// URIs map into it on
// export because an RDF consumer expects a dereferenceable IRI.
const (
	NSHF      = "https://huggingface.co/ns#"
	NSHFR     = "https://huggingface.co/"
	NSSchema  = "https://schema.org/"
	NSCr      = "http://mlcommons.org/croissant/"
	NSDcterms = "http://purl.org/dc/terms/"
	NSFoaf    = "http://xmlns.com/foaf/0.1/"
	NSRdf     = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"
	NSRdfs    = "http://www.w3.org/2000/01/rdf-schema#"
	NSXsd     = "http://www.w3.org/2001/XMLSchema#"
)

var rdfPrefixes = [][2]string{
	{"hf", NSHF},
	{"hfr", NSHFR},
	{"schema", NSSchema},
	{"cr", NSCr},
	{"dcterms", NSDcterms},
	{"foaf", NSFoaf},
	{"rdf", NSRdf},
	{"rdfs", NSRdfs},
	{"xsd", NSXsd},
}

// rdfTypes is the class mapping. The schema alignment is what makes the output
// useful outside this tool.
var rdfTypes = map[string][]string{
	KindModel:      {"hf:Model", "schema:SoftwareApplication"},
	KindDataset:    {"hf:Dataset", "schema:Dataset", "cr:Dataset"},
	KindSpace:      {"hf:Space", "schema:WebApplication"},
	KindKernel:     {"hf:Kernel", "schema:SoftwareSourceCode"},
	KindUser:       {"hf:User", "schema:Person", "foaf:Person"},
	KindOrg:        {"hf:Organization", "schema:Organization"},
	KindCollection: {"hf:Collection", "schema:Collection"},
	KindPaper:      {"hf:Paper", "schema:ScholarlyArticle"},
	KindPost:       {"hf:Post", "schema:SocialMediaPosting"},
	KindBlog:       {"hf:BlogPost", "schema:BlogPosting"},
	KindDiscussion: {"hf:Discussion", "schema:DiscussionForumPosting"},
	KindTag:        {"hf:Tag", "schema:DefinedTerm"},
	KindTask:       {"hf:Task", "schema:DefinedTerm"},
	KindFile:       {"hf:File", "schema:MediaObject"},
	KindSplit:      {"hf:Split", "schema:Dataset"},
	KindCommit:     {"hf:Commit"},
	KindProvider:   {"hf:Provider", "schema:Organization"},
}

// schemaAlias is the standard predicate emitted alongside the hub one.
var schemaAlias = map[string]string{
	PredOwnedBy:      "schema:author",
	PredLicense:      "schema:license",
	PredLastModified: "schema:dateModified",
	PredCreatedAt:    "schema:dateCreated",
	PredPublishedAt:  "schema:datePublished",
	PredHasTag:       "schema:keywords",
	PredCites:        "schema:citation",
	PredDerivesFrom:  "schema:isBasedOn",
	PredContains:     "schema:hasPart",
}

// literalTypes says how a literal predicate's value is typed. A count exported
// as a string is a count nobody can sum.
var literalTypes = map[string]string{
	PredDownloads:     "xsd:integer",
	PredDownloadsAll:  "xsd:integer",
	PredLikeCount:     "xsd:integer",
	PredUsedStorage:   "xsd:integer",
	PredNumParameters: "xsd:integer",
	PredNumRows:       "xsd:integer",
	PredTrendingScore: "xsd:decimal",
	PredCreatedAt:     "xsd:dateTime",
	PredLastModified:  "xsd:dateTime",
	PredPublishedAt:   "xsd:dateTime",
	PredPrivate:       "xsd:boolean",
	PredVerified:      "xsd:boolean",
	PredValue:         "xsd:decimal",
}

// The output formats.
const (
	FormatNT     = "nt"
	FormatTurtle = "ttl"
	FormatJSONLD = "jsonld"
	FormatNQuads = "nq"
)

// RDFOptions controls an export.
type RDFOptions struct {
	Format string
	// Graph is the fourth position for N-Quads. Putting the fetch URL there
	// means the provenance survives into the RDF and a triple store can answer
	// which page told us this.
	Graph string
}

// WriteRDF serialises a graph. N-Triples is the default because it streams, so
// a crawl of a large org never needs the graph in memory.
func WriteRDF(w io.Writer, g *Graph, o RDFOptions) error {
	switch o.Format {
	case "", FormatNT:
		return writeTriples(w, g, "")
	case FormatNQuads:
		return writeTriples(w, g, o.Graph)
	case FormatTurtle:
		return writeTurtle(w, g)
	case FormatJSONLD:
		return writeJSONLD(w, g)
	default:
		return fmt.Errorf("unknown rdf format %q", o.Format)
	}
}

func writeTriples(w io.Writer, g *Graph, graph string) error {
	suffix := " .\n"
	if graph != "" {
		suffix = " <" + graph + "> .\n"
	}
	for _, n := range g.Nodes {
		for _, t := range rdfTypes[n.Kind] {
			if _, err := io.WriteString(w, iri(n.URI)+" <"+NSRdf+"type> "+expand(t)+suffix); err != nil {
				return err
			}
		}
		if n.Label != "" {
			for _, p := range []string{"schema:name", "rdfs:label"} {
				if _, err := io.WriteString(w, iri(n.URI)+" "+expand(p)+" "+quote(n.Label)+suffix); err != nil {
					return err
				}
			}
		}
	}
	for _, e := range g.Edges {
		for _, line := range tripleLines(e) {
			if _, err := io.WriteString(w, line+suffix); err != nil {
				return err
			}
		}
	}
	return nil
}

// tripleLines renders one edge, plus its schema.org twin when it has one.
func tripleLines(e Edge) []string {
	obj := iri(e.Object)
	if e.Literal {
		obj = typedLiteral(e.Predicate, e.Object)
	}
	subj := iri(e.Subject)
	out := []string{subj + " " + expand(e.Predicate) + " " + obj}
	if alias, ok := schemaAlias[e.Predicate]; ok {
		out = append(out, subj+" "+expand(alias)+" "+obj)
	}
	return out
}

// iri renders a subject or object. A blank node stays a blank node, and an
// hf:// URI becomes the https form the hub's own JSON-LD uses.
func iri(uri string) string {
	if strings.HasPrefix(uri, "_:") {
		return uri
	}
	return "<" + IRI(uri) + ">"
}

// IRI maps an hf:// URI to its dereferenceable https form. The hf:// form stays
// in the record plane where it is a stable key rather than a location.
func IRI(uri string) string {
	kind, id, ok := SplitURI(uri)
	if !ok {
		return uri
	}
	if u, err := Locate(kind, id); err == nil {
		return u
	}
	return NSHFR + "ns#" + kind + "/" + id
}

func expand(curie string) string {
	prefix, rest, ok := strings.Cut(curie, ":")
	if !ok {
		return "<" + curie + ">"
	}
	for _, p := range rdfPrefixes {
		if p[0] == prefix {
			return "<" + p[1] + rest + ">"
		}
	}
	return "<" + curie + ">"
}

func typedLiteral(pred, value string) string {
	if t, ok := literalTypes[pred]; ok {
		return quote(value) + "^^" + expand(t)
	}
	return quote(value)
}

func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// writeTurtle groups by subject, which is the whole reason to prefer Turtle: a
// node and everything said about it read as one paragraph.
func writeTurtle(w io.Writer, g *Graph) error {
	for _, p := range rdfPrefixes {
		if _, err := fmt.Fprintf(w, "@prefix %s: <%s> .\n", p[0], p[1]); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	bySubject := map[string][][2]string{}
	var order []string
	add := func(subj, pred, obj string) {
		if _, seen := bySubject[subj]; !seen {
			order = append(order, subj)
		}
		bySubject[subj] = append(bySubject[subj], [2]string{pred, obj})
	}
	for _, n := range g.Nodes {
		for _, t := range rdfTypes[n.Kind] {
			add(n.URI, "a", t)
		}
		if n.Label != "" {
			add(n.URI, "schema:name", quote(n.Label))
			add(n.URI, "rdfs:label", quote(n.Label))
		}
	}
	for _, e := range g.Edges {
		obj := turtleTerm(e.Object)
		if e.Literal {
			obj = turtleLiteral(e.Predicate, e.Object)
		}
		add(e.Subject, e.Predicate, obj)
		if alias, ok := schemaAlias[e.Predicate]; ok {
			add(e.Subject, alias, obj)
		}
	}

	for _, subj := range order {
		if _, err := io.WriteString(w, turtleTerm(subj)+"\n"); err != nil {
			return err
		}
		pairs := bySubject[subj]
		for i, pair := range pairs {
			end := " ;\n"
			if i == len(pairs)-1 {
				end = " .\n\n"
			}
			if _, err := io.WriteString(w, "    "+pair[0]+" "+pair[1]+end); err != nil {
				return err
			}
		}
	}
	return nil
}

func turtleTerm(uri string) string {
	if strings.HasPrefix(uri, "_:") {
		return uri
	}
	return "<" + IRI(uri) + ">"
}

func turtleLiteral(pred, value string) string {
	if t, ok := literalTypes[pred]; ok {
		return quote(value) + "^^" + t
	}
	return quote(value)
}

// writeJSONLD emits one object per node with its edges folded in, and an inline
// context so the document stands alone.
func writeJSONLD(w io.Writer, g *Graph) error {
	ctx := map[string]any{}
	for _, p := range rdfPrefixes {
		ctx[p[0]] = p[1]
	}

	byURI := map[string]map[string]any{}
	var order []string
	obj := func(uri string) map[string]any {
		o, ok := byURI[uri]
		if !ok {
			o = map[string]any{"@id": jsonldID(uri)}
			byURI[uri] = o
			order = append(order, uri)
		}
		return o
	}
	for _, n := range g.Nodes {
		o := obj(n.URI)
		if types := rdfTypes[n.Kind]; len(types) > 0 {
			o["@type"] = types
		}
		if n.Label != "" {
			o["schema:name"] = n.Label
			o["rdfs:label"] = n.Label
		}
		if n.URL != "" {
			o["schema:url"] = n.URL
		}
	}
	for _, e := range g.Edges {
		o := obj(e.Subject)
		var value any
		if e.Literal {
			value = jsonldLiteral(e.Predicate, e.Object)
		} else {
			value = map[string]any{"@id": jsonldID(e.Object)}
		}
		appendValue(o, e.Predicate, value)
		if alias, ok := schemaAlias[e.Predicate]; ok {
			appendValue(o, alias, value)
		}
	}

	graph := make([]map[string]any, 0, len(order))
	for _, uri := range order {
		graph = append(graph, byURI[uri])
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(map[string]any{"@context": ctx, "@graph": graph})
}

// appendValue keeps repeated predicates as a list rather than letting the last
// one win, because a model with twelve tags has twelve of them.
func appendValue(o map[string]any, pred string, value any) {
	switch cur := o[pred].(type) {
	case nil:
		o[pred] = value
	case []any:
		o[pred] = append(cur, value)
	default:
		o[pred] = []any{cur, value}
	}
}

func jsonldID(uri string) string {
	if strings.HasPrefix(uri, "_:") {
		return uri
	}
	return IRI(uri)
}

// jsonldLiteral gives a value its type, so a count arrives as a number and a
// timestamp as a typed value rather than as prose.
func jsonldLiteral(pred, value string) any {
	switch literalTypes[pred] {
	case "xsd:integer":
		if n, err := strconv.ParseInt(value, 10, 64); err == nil {
			return n
		}
	case "xsd:decimal":
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	case "xsd:boolean":
		return value == "true"
	case "xsd:dateTime":
		if t, err := time.Parse(time.RFC3339, value); err == nil {
			return map[string]any{"@value": t.UTC().Format(time.RFC3339), "@type": "xsd:dateTime"}
		}
	}
	return value
}

// MergeCroissant folds the hub's own Croissant document into a JSON-LD export.
// The hub already publishes it correctly, and reimplementing an existing
// standards-body serialisation would be strictly worse than passing it through.
func MergeCroissant(doc json.RawMessage, g *Graph, w io.Writer) error {
	var croissant map[string]any
	if err := json.Unmarshal(doc, &croissant); err != nil {
		return err
	}
	var buf strings.Builder
	if err := writeJSONLD(&buf, g); err != nil {
		return err
	}
	var own map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &own); err != nil {
		return err
	}
	graph, _ := own["@graph"].([]any)
	croissant["hf:graph"] = graph
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(croissant)
}

// SortEdges gives an export a stable order, which is what makes a diff of two
// runs readable.
func SortEdges(edges []Edge) {
	sort.SliceStable(edges, func(i, j int) bool {
		if edges[i].Subject != edges[j].Subject {
			return edges[i].Subject < edges[j].Subject
		}
		if edges[i].Predicate != edges[j].Predicate {
			return edges[i].Predicate < edges[j].Predicate
		}
		return edges[i].Object < edges[j].Object
	})
}
