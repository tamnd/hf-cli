package hf

import (
	"encoding/json"
	"strings"
)

// tag.go turns the hub's flat tag arrays into typed nodes. A repo carries its
// most useful relations as tag strings, so this file is where a large part of
// the graph actually comes from.

// Tag is one entry of the hub's controlled vocabulary. Both the tags-by-type
// documents and the page-embedded tag_objs decode into this.
type Tag struct {
	Meta

	ID      string `json:"id" table:"id"`
	Label   string `json:"label,omitempty" table:"label"`
	Type    string `json:"type,omitempty" table:"type"`
	SubType string `json:"subType,omitempty" table:"subtype"`
	Count   int    `json:"count,omitempty" table:"count"`
	Derived bool   `json:"derived,omitempty" table:"-"`

	// PaperTitle is set on an arxiv tag, and is the only place a repo's tag list
	// says what the paper it cites is actually called.
	PaperTitle string `json:"paperTitle,omitempty" table:"-"`

	// Clickable and Disabled are how the page renders the tag: a disabled tag
	// links nowhere, which on a dataset tag means the dataset it names is gone
	// or private. They are the hub's own judgement about which of a repo's
	// declared relations still resolve.
	Clickable bool `json:"clickable,omitempty" table:"-"`
	Disabled  bool `json:"disabled,omitempty" table:"-"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
//
// A page tag nests some of its fields under a key the hub calls extra, which
// collides with the Extra every record here uses for the opposite purpose. So
// that object is decoded a second time into the same struct: its keys are
// fields hf models, and anything left over lands in Extra, which is where an
// unmodelled field belongs anyway.
func (t *Tag) UnmarshalJSON(b []byte) error {
	type raw Tag
	var all map[string]json.RawMessage
	nested := json.RawMessage(nil)
	if json.Unmarshal(b, &all) == nil && len(all["extra"]) > 0 {
		nested = all["extra"]
		delete(all, "extra")
		if outer, err := json.Marshal(all); err == nil {
			b = outer
		}
	}
	if err := decodeExtra(b, (*raw)(t), &t.Extra); err != nil {
		return err
	}
	if len(nested) == 0 {
		return nil
	}
	var inner Tag
	if err := decodeExtra(nested, (*raw)(&inner), &inner.Extra); err != nil {
		return err
	}
	takeString(&t.PaperTitle, inner.PaperTitle)
	t.Clickable = t.Clickable || inner.Clickable
	t.Disabled = t.Disabled || inner.Disabled
	for k, v := range inner.Extra {
		if t.Extra == nil {
			t.Extra = map[string]json.RawMessage{}
		}
		t.Extra[k] = v
	}
	return nil
}

func (t *Tag) normalize(sourceURL string) {
	if t.Type == "" {
		t.Type = TypeOther
	}
	t.setMeta(KindTag, TagID(t.Type, t.ID), sourceURL)
}

// TagID is the canonical id of a tag node. A tag that already carries its
// namespace keeps it, and a bare word gets its type prefixed, so
// hf://tag/library:transformers and hf://tag/region:us are the same shape.
func TagID(typ, id string) string {
	if typ == "" {
		typ = TypeOther
	}
	if strings.HasPrefix(id, typ+":") {
		return id
	}
	return typ + ":" + id
}

// The tag type vocabulary, as the tags-by-type documents name it.
const (
	TypeLibrary  = "library"
	TypePipeline = "pipeline_tag"
	TypeLanguage = "language"
	TypeLicense  = "license"
	TypeRegion   = "region"
	TypeDataset  = "dataset"
	TypeArxiv    = "arxiv"
	TypeDOI      = "doi"
	TypeBase     = "base_model"
	TypeOther    = "other"
)

// The four base_model relations the hub defines.
const (
	RelFinetune  = "finetune"
	RelAdapter   = "adapter"
	RelQuantized = "quantized"
	RelMerge     = "merge"
)

var baseRelations = map[string]bool{
	RelFinetune:  true,
	RelAdapter:   true,
	RelQuantized: true,
	RelMerge:     true,
}

// Capability flags: bare words the hub sets itself rather than tags an author
// wrote. They stay in other:, but they are worth knowing by name.
var capabilityTags = map[string]bool{
	"endpoints_compatible":      true,
	"custom_code":               true,
	"eval-results":              true,
	"autotrain_compatible":      true,
	"text-generation-inference": true,
	"text-embeddings-inference": true,
	"has_space":                 true,
	"mteb":                      true,
	"not-for-all-audiences":     true,
	"conversational":            true,
}

// ParsedTag is the decode of one raw tag string into its namespace and value.
// This is the function that turns a flat tag array into graph edges.
type ParsedTag struct {
	Raw       string `json:"raw"`
	Namespace string `json:"namespace,omitempty"`
	Relation  string `json:"relation,omitempty"`
	Value     string `json:"value"`
	Kind      string `json:"kind"`
	TargetURI string `json:"targetUri,omitempty"`
}

// TagURI is the address of the tag node this tag always produces, in addition
// to whatever typed edge it may also produce.
func (p ParsedTag) TagURI() string {
	return URI(KindTag, TagID(p.Kind, p.Raw))
}

// ParseTag decodes one raw tag string. The taxonomy may be nil, in which case
// bare words that would have resolved to a library or a task fall through to
// other, which is a degradation and not an error.
//
// Classification order matters: namespaced patterns first, then the taxonomy
// lookup, then other. That order is what makes transformers a library and
// vision-language a free tag.
func ParseTag(raw string, tax *Taxonomy) ParsedTag {
	p := ParsedTag{Raw: raw, Value: raw, Kind: TypeOther}

	ns, rest, hasNS := strings.Cut(raw, ":")
	if hasNS {
		switch ns {
		case TypeLicense:
			p.Namespace, p.Value, p.Kind = ns, rest, TypeLicense
			return p
		case TypeArxiv:
			p.Namespace, p.Value, p.Kind = ns, trimVersion(rest), KindPaper
			p.TargetURI = URI(KindPaper, p.Value)
			return p
		case TypeDOI:
			// A DOI names the repo itself, so it is a literal and not an edge.
			p.Namespace, p.Value, p.Kind = ns, rest, TypeDOI
			return p
		case TypeBase:
			p.Namespace, p.Kind = ns, KindModel
			if rel, target, ok := strings.Cut(rest, ":"); ok && baseRelations[rel] {
				p.Relation, p.Value = rel, target
			} else {
				p.Value = rest
			}
			p.TargetURI = URI(KindModel, p.Value)
			return p
		case TypeDataset:
			p.Namespace, p.Value, p.Kind = ns, rest, KindDataset
			p.TargetURI = URI(KindDataset, p.Value)
			return p
		case TypeRegion:
			p.Namespace, p.Value, p.Kind = ns, rest, TypeRegion
			return p
		case TypeLanguage:
			p.Namespace, p.Value, p.Kind = ns, rest, TypeLanguage
			return p
		}
		// An unknown namespace is still a namespace. Keeping it beats flattening
		// a tag such as trl:SFT into an opaque string.
		p.Namespace, p.Value = ns, rest
		return p
	}

	if capabilityTags[raw] {
		return p
	}
	if t := tax.Lookup(raw); t != "" {
		p.Kind = t
		return p
	}
	return p
}

// ParseTags decodes a whole tag array, preserving order and duplicates. Order
// is preserved because the hub's own order puts the meaningful tags first.
func ParseTags(raws []string, tax *Taxonomy) []ParsedTag {
	out := make([]ParsedTag, 0, len(raws))
	for _, r := range raws {
		out = append(out, ParseTag(r, tax))
	}
	return out
}

// Taxonomy is the fetched controlled vocabulary, one document per repo kind
// merged into one lookup. The client fetches it once per process and caches it
// on disk for a day, because it changes on the order of weeks.
type Taxonomy struct {
	Meta

	// Groups is the document as it arrived, keyed by tag type.
	Groups map[string][]Tag `json:"groups,omitempty" table:"-"`

	// byID answers the only question the parser asks: what type is this bare
	// word. It is built once at load.
	byID map[string]string
}

// UnmarshalJSON accepts the tags-by-type document shape directly, which is a
// bare object of type name to tag array.
func (t *Taxonomy) UnmarshalJSON(b []byte) error {
	var groups map[string][]Tag
	if err := jsonUnmarshal(b, &groups); err != nil {
		return err
	}
	t.Merge(groups)
	return nil
}

// MarshalJSON emits the merged document, so a round trip through the tool
// returns something the tool can read back.
func (t Taxonomy) MarshalJSON() ([]byte, error) {
	type out struct {
		Meta
		Groups map[string][]Tag `json:"groups"`
	}
	return json.Marshal(out{Meta: t.Meta, Groups: t.Groups})
}

// Merge folds another tags-by-type document into this taxonomy. The model and
// dataset documents overlap on language and license and disagree on nothing,
// so a later document only ever adds.
func (t *Taxonomy) Merge(groups map[string][]Tag) {
	if t.Groups == nil {
		t.Groups = make(map[string][]Tag, len(groups))
	}
	if t.byID == nil {
		t.byID = make(map[string]string)
	}
	for typ, tags := range groups {
		seen := make(map[string]bool, len(t.Groups[typ]))
		for _, existing := range t.Groups[typ] {
			seen[existing.ID] = true
		}
		for _, tag := range tags {
			if tag.Type == "" {
				tag.Type = typ
			}
			if !seen[tag.ID] {
				t.Groups[typ] = append(t.Groups[typ], tag)
				seen[tag.ID] = true
			}
			// A namespaced id such as region:us is already unambiguous and does
			// not belong in the bare word index.
			if _, _, hasNS := strings.Cut(tag.ID, ":"); !hasNS {
				if _, dup := t.byID[tag.ID]; !dup {
					t.byID[tag.ID] = typ
				}
			}
		}
	}
}

// Lookup answers the type of a bare tag word, or empty when the taxonomy does
// not know it. A nil taxonomy answers empty rather than panicking, so tag
// parsing works before the vocabulary has been fetched.
func (t *Taxonomy) Lookup(id string) string {
	if t == nil || t.byID == nil {
		return ""
	}
	return t.byID[id]
}

// Tags flattens the taxonomy into one stream, which is what the list command
// emits.
func (t *Taxonomy) Tags() []Tag {
	if t == nil {
		return nil
	}
	n := 0
	for _, g := range t.Groups {
		n += len(g)
	}
	out := make([]Tag, 0, n)
	for typ, g := range t.Groups {
		for _, tag := range g {
			if tag.Type == "" {
				tag.Type = typ
			}
			tag.normalize("")
			out = append(out, tag)
		}
	}
	return out
}

// Task is one entry of /api/tasks: the hub's curated view of a machine learning
// task, with the models, datasets, and spaces its editors chose. Those picks
// are editorial, which makes them a different and stronger signal than a
// download count.
type Task struct {
	Meta

	ID            string          `json:"id" table:"id"`
	Label         string          `json:"label" table:"label"`
	Summary       string          `json:"summary,omitempty" table:"summary,truncate"`
	Libraries     []string        `json:"libraries,omitempty" table:"-"`
	Models        []TaskItem      `json:"models,omitempty" table:"-"`
	Datasets      []TaskItem      `json:"datasets,omitempty" table:"-"`
	Spaces        []TaskItem      `json:"spaces,omitempty" table:"-"`
	Metrics       []TaskItem      `json:"metrics,omitempty" table:"-"`
	WidgetModels  []string        `json:"widgetModels,omitempty" table:"-"`
	YoutubeID     string          `json:"youtubeId,omitempty" table:"-"`
	Demo          json.RawMessage `json:"demo,omitempty" table:"-"`
	IsPlaceholder bool            `json:"isPlaceholder,omitempty" table:"-"`

	// CanonicalID names the task this one is an alias of. Several task pages are
	// synonyms that redirect, and without this the alias looks like a task in its
	// own right with nothing in it.
	CanonicalID string `json:"canonicalId,omitempty" table:"-"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (t *Task) UnmarshalJSON(b []byte) error {
	type raw Task
	return decodeExtra(b, (*raw)(t), &t.Extra)
}

func (t *Task) normalize(sourceURL string) {
	for i := range t.Models {
		t.Models[i].normalize(KindModel)
	}
	for i := range t.Datasets {
		t.Datasets[i].normalize(KindDataset)
	}
	for i := range t.Spaces {
		t.Spaces[i].normalize(KindSpace)
	}
	for i := range t.Metrics {
		t.Metrics[i].normalize("")
	}
	t.setMeta(KindTask, t.ID, sourceURL)
}

// TaskItem is one editorial pick, with the editor's own one line reason.
type TaskItem struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	URI         string `json:"uri,omitempty"`
}

func (i *TaskItem) normalize(kind string) {
	if kind == "" || i.ID == "" {
		return
	}
	i.URI = URI(kind, i.ID)
}
