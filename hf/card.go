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
	License     StringList `json:"license,omitempty"`
	LicenseName string     `json:"license_name,omitempty"`
	LicenseLink string     `json:"license_link,omitempty"`
	Language    StringList `json:"language,omitempty"`
	Tags        StringList `json:"tags,omitempty"`
	Datasets    StringList `json:"datasets,omitempty"`
	Metrics     StringList `json:"metrics,omitempty"`
	Thumbnail   string     `json:"thumbnail,omitempty"`
	PrettyName  string     `json:"pretty_name,omitempty"`
	Viewer      *bool      `json:"viewer,omitempty"`
	DOI         string     `json:"doi,omitempty"`

	// Model.
	BaseModel         StringList      `json:"base_model,omitempty"`
	BaseModelRelation string          `json:"base_model_relation,omitempty"`
	PipelineTag       string          `json:"pipeline_tag,omitempty"`
	LibraryName       string          `json:"library_name,omitempty"`
	ModelIndex        json.RawMessage `json:"model-index,omitempty"`
	Inference         json.RawMessage `json:"inference,omitempty"`
	Widget            json.RawMessage `json:"widget,omitempty"`
	NewVersion        string          `json:"new_version,omitempty"`
	ExtraGatedPrompt  string          `json:"extra_gated_prompt,omitempty"`
	CO2Emissions      json.RawMessage `json:"co2_eq_emissions,omitempty"`
	DuplicatedFrom    string          `json:"duplicated_from,omitempty"`
	Quantized         json.RawMessage `json:"quantized_by,omitempty"`

	// Dataset.
	TaskCategories      StringList      `json:"task_categories,omitempty"`
	TaskIDs             StringList      `json:"task_ids,omitempty"`
	AnnotationsCreators StringList      `json:"annotations_creators,omitempty"`
	LanguageCreators    StringList      `json:"language_creators,omitempty"`
	Multilinguality     StringList      `json:"multilinguality,omitempty"`
	SizeCategories      StringList      `json:"size_categories,omitempty"`
	SourceDatasets      StringList      `json:"source_datasets,omitempty"`
	Configs             json.RawMessage `json:"configs,omitempty"`
	DatasetInfo         json.RawMessage `json:"dataset_info,omitempty"`
	TrainEvalIndex      json.RawMessage `json:"train-eval-index,omitempty"`

	// Space.
	Title             string     `json:"title,omitempty"`
	Emoji             string     `json:"emoji,omitempty"`
	ColorFrom         string     `json:"colorFrom,omitempty"`
	ColorTo           string     `json:"colorTo,omitempty"`
	SDK               string     `json:"sdk,omitempty"`
	SDKVersion        string     `json:"sdk_version,omitempty"`
	PythonVersion     string     `json:"python_version,omitempty"`
	AppFile           string     `json:"app_file,omitempty"`
	AppPort           int        `json:"app_port,omitempty"`
	BasePath          string     `json:"base_path,omitempty"`
	Pinned            bool       `json:"pinned,omitempty"`
	ShortDescription  string     `json:"short_description,omitempty"`
	Models            StringList `json:"models,omitempty"`
	SuggestedHardware string     `json:"suggested_hardware,omitempty"`
	Header            string     `json:"header,omitempty"`
	Disabled          bool       `json:"disabled,omitempty"`

	// Extra is every key hf does not model, and there are many: per org
	// conventions, experiment trackers, and whatever a template happened to
	// include.
	Extra map[string]json.RawMessage `json:"-"`
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
