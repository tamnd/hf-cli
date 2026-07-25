// Package hf is the library behind the hf command line: the HTTP client for
// huggingface.co, the typed records for everything the hub publishes, and the
// graph those records form.
//
// The hub is already a knowledge graph served as web pages. A model declares
// the dataset it was trained on, the paper it implements, the model it was
// fine-tuned from, and the license it ships under. A space declares the models
// it loads. A collection curates across all of them. This package reads those
// declarations, gives each entity a stable hf:// URI, and emits both the
// records and the edges between them.
//
// The spec lives in ~/notes/Spec/3001.
package hf

import (
	"encoding/json"
	"strings"
	"time"
)

// Host is the site this client talks to.
const Host = "huggingface.co"

// BaseURL is the root every hub request is built from.
const BaseURL = "https://" + Host

// ViewerURL is the dataset-viewer service. It is a separate host with its own
// API, and several dataset endpoints exist only there.
const ViewerURL = "https://datasets-server.huggingface.co"

// Scheme is the URI scheme for hub resources: hf://model/owner/name.
const Scheme = "hf"

// DefaultUserAgent identifies the client. An honest User-Agent is both polite
// and the thing most likely to keep you unblocked.
const DefaultUserAgent = "hf-cli/dev (+https://github.com/tamnd/hf-cli)"

// The entity kinds. Every URI names one of these, and the set is closed: a new
// kind means a new constant here, a Locate case, and a Classify rule.
const (
	KindModel      = "model"
	KindDataset    = "dataset"
	KindSpace      = "space"
	KindKernel     = "kernel"
	KindUser       = "user"
	KindOrg        = "org"
	KindNamespace  = "namespace" // a name we know is one of user or org, but not which
	KindCollection = "collection"
	KindPaper      = "paper"
	KindPost       = "post"
	KindBlog       = "blog"
	KindDiscussion = "discussion"
	KindCommit     = "commit"
	KindRef        = "ref"
	KindFile       = "file"
	KindTag        = "tag"
	KindTask       = "task"
	KindSplit      = "split"
	KindProvider   = "provider"
)

// RepoKinds are the four kinds that live at /{owner}/{name} and share the Repo
// shape.
var RepoKinds = []string{KindModel, KindDataset, KindSpace, KindKernel}

// Meta is embedded in every record. It is the part of a record hf adds rather
// than reads, and it is what makes a record addressable and auditable.
//
// The table tags run through every record type in this package. A record models
// everything the hub said, which is far more than fits across a terminal, so
// each type marks the handful of fields worth a column and hides the rest with
// table:"-". Hidden means hidden from the table, csv, and tsv views only: json
// and jsonl still carry the whole record, and --fields names any column back
// into view.
type Meta struct {
	// URI is the canonical hf:// address, and the store's primary key.
	URI string `json:"uri" kit:"id" table:"-"`
	// URL is where the entity lives on the web. It is the canonical address for
	// -o url and stays out of the table, because every record already leads with
	// an id that says the same thing in a third of the width.
	URL string `json:"url,omitempty" table:"-,url"`
	// Kind is one of the Kind constants.
	Kind string `json:"kind,omitempty" table:"-"`
	// Sources lists every URL that contributed a field to this record, so a
	// surprising value can always be traced back to what said it.
	Sources []string `json:"sources,omitempty" table:"-"`
	// FetchedAt is when the record was assembled.
	FetchedAt time.Time `json:"fetchedAt,omitzero" table:"-"`
	// AliasOf is the id the caller asked for, when it differed from the
	// canonical one. Asking for bert-base-uncased yields a record whose id is
	// google-bert/bert-base-uncased and whose aliasOf is what was typed.
	AliasOf string `json:"aliasOf,omitempty" table:"-"`
	// Extra holds every field the upstream response carried that this version
	// of hf does not model. It stays raw so a large integer survives the round
	// trip, and it is never dropped, because a field hf has not seen is exactly
	// the field worth noticing.
	Extra map[string]json.RawMessage `json:"extra,omitempty" table:"-"`
}

// setMeta fills the computed fields. Every constructor and decode path ends
// here, so no record escapes without an address.
func (m *Meta) setMeta(kind, id string, sources ...string) {
	m.Kind = kind
	m.URI = URI(kind, id)
	if u, err := Locate(kind, id); err == nil {
		m.URL = u
	}
	if m.FetchedAt.IsZero() {
		m.FetchedAt = time.Now().UTC()
	}
	for _, s := range sources {
		m.addSource(s)
	}
}

// addSource records one contributing URL, skipping duplicates so a merge of
// several fetches does not repeat itself.
func (m *Meta) addSource(url string) {
	if url == "" {
		return
	}
	for _, s := range m.Sources {
		if s == url {
			return
		}
	}
	m.Sources = append(m.Sources, url)
}

// Gated is the repo access state: not gated, or gated with automatic or manual
// approval. Upstream sends false, "auto", or "manual" in the same field, so the
// type decodes from a bool or a string.
type Gated string

// The gate states.
const (
	NotGated    Gated = ""
	GatedAuto   Gated = "auto"
	GatedManual Gated = "manual"
)

// UnmarshalJSON accepts false, true, "auto", and "manual".
func (g *Gated) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	switch s {
	case "false", "null", "":
		*g = NotGated
	case "true":
		*g = GatedAuto
	default:
		*g = Gated(s)
	}
	return nil
}

// IsGated reports whether the repo requires approval.
func (g Gated) IsGated() bool { return g != NotGated }

// StringList decodes a value that is either a scalar string or a list of
// strings. Card authors write `license: mit` and `license: [mit]` for the same
// key, sometimes in the same repository, and both are valid on the hub, so a
// parser that accepts only one form fails on a large share of real repos.
type StringList []string

// UnmarshalJSON accepts a string, a list of strings, or null.
func (s *StringList) UnmarshalJSON(b []byte) error {
	b = trimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		var list []any
		if err := jsonUnmarshal(b, &list); err != nil {
			return err
		}
		out := make([]string, 0, len(list))
		for _, v := range list {
			if str, ok := stringOf(v); ok && str != "" {
				out = append(out, str)
			}
		}
		*s = out
		return nil
	}
	var one any
	if err := jsonUnmarshal(b, &one); err != nil {
		return err
	}
	if str, ok := stringOf(one); ok && str != "" {
		*s = StringList{str}
	}
	return nil
}
