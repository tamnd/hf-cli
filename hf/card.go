package hf

import (
	"encoding/json"
	"strings"

	"sigs.k8s.io/yaml"
)

// card.go parses the YAML front matter of a repo README. It is the single
// richest declarative source on the hub and also the messiest, because it is
// author written and only partly validated. Every rule here exists because real
// repos break the obvious version of it.

// Card is the parsed front matter. Every field is optional, and the list fields
// use StringList because authors write both the scalar and the list form for
// the same key.
type Card struct {
	// Universal.
	License     StringList `json:"license,omitempty" table:"license"`
	LicenseName string     `json:"license_name,omitempty" table:"-"`
	LicenseLink string     `json:"license_link,omitempty" table:"-"`
	Language    StringList `json:"language,omitempty" table:"-"`
	Tags        StringList `json:"tags,omitempty" table:"tags"`
	Datasets    StringList `json:"datasets,omitempty" table:"-"`
	Metrics     StringList `json:"metrics,omitempty" table:"-"`
	Thumbnail   string     `json:"thumbnail,omitempty" table:"-"`
	PrettyName  string     `json:"pretty_name,omitempty" table:"-"`
	Viewer      *bool      `json:"viewer,omitempty" table:"-"`
	DOI         string     `json:"doi,omitempty" table:"-"`

	// PapersWithCodeID links the repo to its Papers with Code entry. It is a
	// card key the hub reads rather than an author convention, which is why it
	// is also an expand field on the API.
	PapersWithCodeID string `json:"paperswithcode_id,omitempty" table:"-"`

	// Model.
	BaseModel         StringList      `json:"base_model,omitempty" table:"base_model"`
	BaseModelRelation string          `json:"base_model_relation,omitempty" table:"-"`
	PipelineTag       string          `json:"pipeline_tag,omitempty" table:"task"`
	LibraryName       string          `json:"library_name,omitempty" table:"library"`
	ModelIndex        json.RawMessage `json:"model-index,omitempty" table:"-"`
	Inference         json.RawMessage `json:"inference,omitempty" table:"-"`
	Widget            json.RawMessage `json:"widget,omitempty" table:"-"`
	NewVersion        string          `json:"new_version,omitempty" table:"-"`
	ExtraGatedPrompt  string          `json:"extra_gated_prompt,omitempty" table:"-"`
	CO2Emissions      json.RawMessage `json:"co2_eq_emissions,omitempty" table:"-"`
	DuplicatedFrom    string          `json:"duplicated_from,omitempty" table:"-"`
	Quantized         json.RawMessage `json:"quantized_by,omitempty" table:"-"`

	// Dataset.
	TaskCategories      StringList      `json:"task_categories,omitempty" table:"-"`
	TaskIDs             StringList      `json:"task_ids,omitempty" table:"-"`
	AnnotationsCreators StringList      `json:"annotations_creators,omitempty" table:"-"`
	LanguageCreators    StringList      `json:"language_creators,omitempty" table:"-"`
	Multilinguality     StringList      `json:"multilinguality,omitempty" table:"-"`
	SizeCategories      StringList      `json:"size_categories,omitempty" table:"-"`
	SourceDatasets      StringList      `json:"source_datasets,omitempty" table:"-"`
	Configs             json.RawMessage `json:"configs,omitempty" table:"-"`
	DatasetInfo         json.RawMessage `json:"dataset_info,omitempty" table:"-"`
	TrainEvalIndex      json.RawMessage `json:"train-eval-index,omitempty" table:"-"`

	// Space.
	Title             string     `json:"title,omitempty" table:"-"`
	Emoji             string     `json:"emoji,omitempty" table:"-"`
	ColorFrom         string     `json:"colorFrom,omitempty" table:"-"`
	ColorTo           string     `json:"colorTo,omitempty" table:"-"`
	SDK               string     `json:"sdk,omitempty" table:"-"`
	SDKVersion        string     `json:"sdk_version,omitempty" table:"-"`
	PythonVersion     string     `json:"python_version,omitempty" table:"-"`
	AppFile           string     `json:"app_file,omitempty" table:"-"`
	AppPort           int        `json:"app_port,omitempty" table:"-"`
	BasePath          string     `json:"base_path,omitempty" table:"-"`
	Pinned            bool       `json:"pinned,omitempty" table:"-"`
	ShortDescription  string     `json:"short_description,omitempty" table:"-"`
	Models            StringList `json:"models,omitempty" table:"-"`
	SuggestedHardware string     `json:"suggested_hardware,omitempty" table:"-"`
	Header            string     `json:"header,omitempty" table:"-"`
	Disabled          bool       `json:"disabled,omitempty" table:"-"`

	// Extra is every key hf does not model, and there are many: per org
	// conventions, experiment trackers, and whatever a template happened to
	// include.
	Extra map[string]json.RawMessage `json:"-" table:"-"`
}

// UnmarshalJSON decodes the known keys and keeps the rest.
func (c *Card) UnmarshalJSON(b []byte) error {
	type raw Card
	return decodeExtra(b, (*raw)(c), &c.Extra)
}

// MarshalJSON folds the unknown keys back in at the top level. A card is
// someone else's document, so a round trip should return it, not reorganise it.
func (c Card) MarshalJSON() ([]byte, error) {
	type raw Card
	return marshalWithExtra(raw(c), c.Extra)
}

// SplitCard separates the YAML front matter from the body. The block counts
// only when the file opens with a fence, which is the rule the hub itself uses.
func SplitCard(readme string) (front, body string) {
	s := strings.TrimLeft(readme, "\ufeff \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return "", readme
	}
	rest := s[3:]
	if i := strings.IndexAny(rest, "\r\n"); i >= 0 {
		rest = rest[i+1:]
	} else {
		return "", readme
	}
	for _, fence := range []string{"\n---\n", "\n---\r\n", "\n...\n"} {
		if j := strings.Index(rest, fence); j >= 0 {
			return rest[:j], strings.TrimLeft(rest[j+len(fence):], "\r\n")
		}
	}
	// An opening fence with no closing one: treat the whole file as body rather
	// than swallowing it as metadata.
	return "", readme
}

// ParseCard parses front matter into a Card. A YAML failure is reported but is
// never fatal: malformed front matter is common on the hub, and it must not
// take down a crawl. The caller puts the message in Repo.CardError and carries
// on with the rest of the record.
func ParseCard(front string) (*Card, error) {
	front = strings.TrimSpace(front)
	if front == "" {
		return nil, nil
	}
	// yaml.YAMLToJSON converts first, so the json tags above are the only tag
	// set the type needs.
	j, err := yaml.YAMLToJSON([]byte(front))
	if err != nil {
		return nil, err
	}
	if len(j) == 0 || string(j) == "null" {
		return nil, nil
	}
	var c Card
	if err := json.Unmarshal(j, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// ParseReadme is the whole path in one call: split, parse, return the body.
func ParseReadme(readme string) (card *Card, body string, err error) {
	front, body := SplitCard(readme)
	card, err = ParseCard(front)
	return card, body, err
}

// LicenseID is the single license value most repos have, which is what the
// license predicate and the license column want.
func (c *Card) LicenseID() string {
	if c == nil {
		return ""
	}
	if len(c.License) > 0 {
		return c.License[0]
	}
	return c.LicenseName
}

// BaseModelRefs turns the card's base_model declaration into the same shape the
// API's baseModels expand uses, so a consumer does not care which source the
// lineage came from.
func (c *Card) BaseModelRefs() []BaseModelRef {
	if c == nil {
		return nil
	}
	rel := c.BaseModelRelation
	out := make([]BaseModelRef, 0, len(c.BaseModel))
	for _, id := range c.BaseModel {
		out = append(out, BaseModelRef{ID: id, Relation: rel})
	}
	return out
}
