package hf

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

// repo.go holds the shape the four repository kinds share. It is a struct
// rather than an interface because callers want the fields, not a method set.

// Repo is the part every repository kind has. Model, Dataset, Space, and Kernel
// each embed it and add their own.
type Repo struct {
	Meta

	ID       string `json:"id"`
	ObjectID string `json:"_id,omitempty"`
	Author   string `json:"author,omitempty"`
	Name     string `json:"name,omitempty"`
	RepoType string `json:"repoType,omitempty"`
	SHA      string `json:"sha,omitempty"`

	Private  bool   `json:"private"`
	Disabled bool   `json:"disabled,omitempty"`
	Gated    Gated  `json:"gated,omitempty"`
	Region   string `json:"region,omitempty"`

	Likes         int     `json:"likes"`
	Downloads     int     `json:"downloads"`
	DownloadsAll  int     `json:"downloadsAllTime,omitempty"`
	TrendingScore float64 `json:"trendingScore,omitempty"`
	UsedStorage   int64   `json:"usedStorage,omitempty"`

	CreatedAt    time.Time `json:"createdAt,omitzero"`
	LastModified time.Time `json:"lastModified,omitzero"`

	Tags     []string  `json:"tags,omitempty"`
	Siblings []Sibling `json:"siblings,omitempty"`

	CardData  *Card  `json:"cardData,omitempty"`
	CardText  string `json:"cardText,omitempty"`
	CardError string `json:"cardError,omitempty"`

	AuthorData    *UserRef       `json:"authorData,omitempty"`
	XetEnabled    bool           `json:"xetEnabled,omitempty"`
	ResourceGroup *ResourceGroup `json:"resourceGroup,omitempty"`

	// IsLikedByUser is relative to the token making the request, so it is always
	// false for an anonymous read.
	IsLikedByUser bool `json:"isLikedByUser,omitempty"`

	// Page-derived, present with --deep.
	CardExists          bool             `json:"cardExists,omitempty"`
	DiscussionsStats    *DiscussionStats `json:"discussionsStats,omitempty"`
	DiscussionsDisabled bool             `json:"discussionsDisabled,omitempty"`
	DiscussionsSorting  string           `json:"discussionsSorting,omitempty"`
	HasBlockedOIDs      bool             `json:"hasBlockedOids,omitempty"`
	LicenseFilePath     string           `json:"licenseFilePath,omitempty"`
	InferenceStatus     string           `json:"inference,omitempty"`

	// TagObjs is the typed form of Tags. Only the page carries it, so it is the
	// one field where the page always wins, and when no page was fetched hf
	// synthesises it from the raw strings and marks each one derived.
	TagObjs   []Tag  `json:"tag_objs,omitempty"`
	Thumbnail string `json:"thumbnail,omitempty"`

	// CardOutline is the README heading tree the page's side navigation renders
	// from, which is a table of contents nothing else on the hub exposes.
	CardOutline json.RawMessage `json:"cardOutline,omitempty"`
	JSONLD      json.RawMessage `json:"jsonld,omitempty"`
}

// Sibling is one entry of the flat file list. The upstream key is rfilename,
// and it is kept as a struct rather than flattened to a string so a reader of
// the raw JSON sees the same field name the API used.
type Sibling struct {
	Filename string `json:"rfilename"`
}

// ResourceGroup is the enterprise access control group a repo belongs to.
type ResourceGroup struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// DiscussionStats is the open and closed count shown on a repo's community tab.
type DiscussionStats struct {
	Open   int `json:"open"`
	Closed int `json:"closed"`
	Total  int `json:"total"`
}

// normalize fills the fields the hub leaves implicit: the owner and name halves
// of the id, the repo kind, and the addressing envelope.
func (r *Repo) normalize(kind, sourceURL string) {
	r.RepoType = kind
	if r.Author == "" || r.Name == "" {
		if owner, name, ok := strings.Cut(r.ID, "/"); ok {
			if r.Author == "" {
				r.Author = owner
			}
			if r.Name == "" {
				r.Name = name
			}
		} else {
			r.Name = r.ID
		}
	}
	if r.AuthorData != nil {
		r.AuthorData.normalize()
	}
	r.setMeta(kind, r.ID, sourceURL)
}

// Files returns the sibling paths as plain strings, which is what most callers
// actually want.
func (r *Repo) Files() []string {
	out := make([]string, 0, len(r.Siblings))
	for _, s := range r.Siblings {
		out = append(out, s.Filename)
	}
	return out
}

// Model is a model repository.
type Model struct {
	Repo

	PipelineTag string `json:"pipeline_tag,omitempty"`
	Library     string `json:"library_name,omitempty"`
	MaskToken   string `json:"mask_token,omitempty"`

	Config           *ModelConfig      `json:"config,omitempty"`
	TransformersInfo *TransformersInfo `json:"transformersInfo,omitempty"`
	Safetensors      *Safetensors      `json:"safetensors,omitempty"`
	GGUF             *GGUF             `json:"gguf,omitempty"`

	ModelIndex  json.RawMessage `json:"model-index,omitempty"`
	EvalResults []EvalResult    `json:"evalResults,omitempty"`

	Spaces             []string           `json:"spaces,omitempty"`
	BaseModels         BaseModels         `json:"baseModels,omitempty"`
	ChildrenCount      *ChildCounts       `json:"childrenModelCount,omitempty"`
	WidgetData         json.RawMessage    `json:"widgetData,omitempty"`
	InferenceProviders InferenceProviders `json:"inferenceProviderMapping,omitempty"`

	// Page-derived.
	LibrariesOther   []string      `json:"librariesOther,omitempty"`
	IsQuantized      bool          `json:"isQuantized,omitempty"`
	HasQuantizations bool          `json:"hasQuantizations,omitempty"`
	LinkedSpaces     []LinkedSpace `json:"linkedSpaces,omitempty"`
	TrackDownloads   bool          `json:"trackDownloads,omitempty"`
	ShowHuggingChat  bool          `json:"showHuggingChatEntry,omitempty"`
	NumParameters    int64         `json:"numParameters,omitempty"`
	WidgetOutputURLs []string      `json:"widgetOutputUrls,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra. It
// folds availableInferenceProviders into the same field as
// inferenceProviderMapping, because they are one list under two names: the
// detail route sends the mapping and every summary row sends the array.
func (m *Model) UnmarshalJSON(b []byte) error {
	type raw Model
	if err := decodeExtra(b, (*raw)(m), &m.Extra, "modelId", "availableInferenceProviders"); err != nil {
		return err
	}
	if len(m.InferenceProviders) == 0 {
		var alt struct {
			Available InferenceProviders `json:"availableInferenceProviders"`
		}
		if json.Unmarshal(b, &alt) == nil {
			m.InferenceProviders = alt.Available
		}
	}
	return nil
}

// ChildCounts is how many models derive from this one, split by relation. The
// hub returns an object rather than a number here, which is more useful than it
// looks: the four keys are exactly the four queries hf children runs, so the
// counts say in advance how big that command's answer will be.
type ChildCounts struct {
	Finetune  int `json:"finetune,omitempty"`
	Adapter   int `json:"adapter,omitempty"`
	Quantized int `json:"quantized,omitempty"`
	Merge     int `json:"merge,omitempty"`

	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps any relation the hub adds later, since the set has grown
// once already.
func (c *ChildCounts) UnmarshalJSON(b []byte) error {
	type raw ChildCounts
	return decodeExtra(b, (*raw)(c), &c.Extra)
}

// Total is what a person means by "how many children".
func (c *ChildCounts) Total() int {
	if c == nil {
		return 0
	}
	return c.Finetune + c.Adapter + c.Quantized + c.Merge
}

// ModelConfig is the subset of config.json the hub itself indexes. The whole
// file is one hf cat away.
type ModelConfig struct {
	Architectures   []string        `json:"architectures,omitempty"`
	ModelType       string          `json:"model_type,omitempty"`
	TokenizerConfig json.RawMessage `json:"tokenizer_config,omitempty"`
	Quantization    json.RawMessage `json:"quantization_config,omitempty"`

	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON keeps the rest of config.json rather than discarding it, since
// what the hub indexes today is a moving target.
func (c *ModelConfig) UnmarshalJSON(b []byte) error {
	type raw ModelConfig
	return decodeExtra(b, (*raw)(c), &c.Extra)
}

// MarshalJSON writes the typed fields with the unknown ones folded back in at
// the top level, so a round trip through hf does not reorganise config.json.
func (c ModelConfig) MarshalJSON() ([]byte, error) {
	type raw ModelConfig
	return marshalWithExtra(raw(c), c.Extra)
}

// TransformersInfo is how the transformers library should load this model.
type TransformersInfo struct {
	AutoModel   string `json:"auto_model,omitempty"`
	PipelineTag string `json:"pipeline_tag,omitempty"`
	Processor   string `json:"processor,omitempty"`
}

// Safetensors is the parameter census. Parameters maps a dtype to a count, so a
// mixed precision model reports each precision separately.
type Safetensors struct {
	Parameters    map[string]int64 `json:"parameters,omitempty"`
	Total         int64            `json:"total,omitempty"`
	Sharded       bool             `json:"sharded,omitempty"`
	TotalFileSize int64            `json:"totalFileSize,omitempty"`
}

// GGUF is the quantized-format metadata for llama.cpp style files.
type GGUF struct {
	Total         int64  `json:"total,omitempty"`
	Architecture  string `json:"architecture,omitempty"`
	ContextLength int    `json:"context_length,omitempty"`
	ChatTemplate  string `json:"chat_template,omitempty"`
	BOSToken      string `json:"bos_token,omitempty"`
	EOSToken      string `json:"eos_token,omitempty"`
	TotalFileSize int64  `json:"totalFileSize,omitempty"`

	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON sweeps the unmodelled gguf keys into Extra.
func (g *GGUF) UnmarshalJSON(b []byte) error {
	type raw GGUF
	return decodeExtra(b, (*raw)(g), &g.Extra)
}

// MarshalJSON folds the unknown gguf keys back to the top level.
func (g GGUF) MarshalJSON() ([]byte, error) {
	type raw GGUF
	return marshalWithExtra(raw(g), g.Extra)
}

// BaseModelRef is a model this one derives from, and how.
type BaseModelRef struct {
	ID       string `json:"id"`
	ObjectID string `json:"_id,omitempty"`
	Relation string `json:"relation,omitempty"`
}

// BaseModels is the ancestry list, which arrives in three shapes. Some routes
// send a flat array with the relation inside each entry, the model detail route
// sends one relation with its models grouped under it, and a model with mixed
// ancestry sends an object keyed by relation. All three say the same thing, and
// the flat array is the one you can sort and turn into edges, so that is what
// this always produces.
type BaseModels []BaseModelRef

func (b *BaseModels) UnmarshalJSON(raw []byte) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '[' {
		var list []BaseModelRef
		if err := json.Unmarshal(raw, &list); err != nil {
			return err
		}
		*b = list
		return nil
	}
	var grouped struct {
		Relation string         `json:"relation"`
		Models   []BaseModelRef `json:"models"`
	}
	if err := json.Unmarshal(raw, &grouped); err == nil && len(grouped.Models) > 0 {
		for i := range grouped.Models {
			if grouped.Models[i].Relation == "" {
				grouped.Models[i].Relation = grouped.Relation
			}
		}
		*b = grouped.Models
		return nil
	}
	var byRelation map[string][]BaseModelRef
	if err := json.Unmarshal(raw, &byRelation); err != nil {
		return err
	}
	var list []BaseModelRef
	for relation, models := range byRelation {
		for _, m := range models {
			if m.Relation == "" {
				m.Relation = relation
			}
			list = append(list, m)
		}
	}
	// Map order is random and a record that changes between two identical runs
	// is a record nobody can diff.
	sort.Slice(list, func(i, j int) bool {
		if list[i].Relation != list[j].Relation {
			return list[i].Relation < list[j].Relation
		}
		return list[i].ID < list[j].ID
	})
	*b = list
	return nil
}

// LinkedSpace is a space that loads this repo, with the live state the page
// knows and the API does not.
type LinkedSpace struct {
	ID               string `json:"id"`
	Emoji            string `json:"emoji,omitempty"`
	Running          bool   `json:"running"`
	Featured         bool   `json:"featured,omitempty"`
	ShortDescription string `json:"shortDescription,omitempty"`
}

// InferenceProvider is one third party serving this model, and under what id.
type InferenceProvider struct {
	Provider           string `json:"provider"`
	Status             string `json:"status,omitempty"`
	ProviderID         string `json:"providerId,omitempty"`
	Task               string `json:"task,omitempty"`
	Adapter            string `json:"adapter,omitempty"`
	AdapterWeightsPath string `json:"adapterWeightsPath,omitempty"`
	IsModelAuthor      bool   `json:"isModelAuthor,omitempty"`
}

// InferenceProviders is a list of providers that arrives in two shapes. The API
// returns an object keyed by provider name and the rendered page returns an
// array with the name inside each entry. Both mean the same thing, so this
// reads either and always presents the array, which is the shape you can sort.
type InferenceProviders []InferenceProvider

func (p *InferenceProviders) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '[' {
		var list []InferenceProvider
		if err := json.Unmarshal(b, &list); err != nil {
			return err
		}
		*p = list
		return nil
	}
	var byName map[string]InferenceProvider
	if err := json.Unmarshal(b, &byName); err != nil {
		return err
	}
	list := make([]InferenceProvider, 0, len(byName))
	for name, entry := range byName {
		if entry.Provider == "" {
			entry.Provider = name
		}
		list = append(list, entry)
	}
	// Map order is random and a record that changes between two identical runs
	// is a record nobody can diff.
	sort.Slice(list, func(i, j int) bool { return list[i].Provider < list[j].Provider })
	*p = list
	return nil
}

// EvalResult is one row of the model index: a task, a dataset, a metric, and a
// number. A card entry with three metrics becomes three of these, because a
// flat row is what you sort, compare, and turn into a triple. The nested
// original stays in Model.ModelIndex.
type EvalResult struct {
	Task          string  `json:"task,omitempty"`
	TaskType      string  `json:"taskType,omitempty"`
	Dataset       string  `json:"dataset,omitempty"`
	DatasetType   string  `json:"datasetType,omitempty"`
	DatasetConfig string  `json:"datasetConfig,omitempty"`
	DatasetSplit  string  `json:"datasetSplit,omitempty"`
	MetricName    string  `json:"metricName,omitempty"`
	MetricType    string  `json:"metricType,omitempty"`
	Value         float64 `json:"value,omitempty"`
	ValueText     string  `json:"valueText,omitempty"`
	Verified      bool    `json:"verified,omitempty"`
	SourceName    string  `json:"sourceName,omitempty"`
	SourceURL     string  `json:"sourceUrl,omitempty"`
}

// Dataset is a dataset repository. The card fields are promoted to the top
// level because for a dataset the card is the description rather than metadata
// about it, and task_categories, language, and size_categories are what anyone
// querying datasets actually filters on. They stay in CardData too.
type Dataset struct {
	Repo

	Description      string `json:"description,omitempty"`
	Citation         string `json:"citation,omitempty"`
	PaperswithcodeID string `json:"paperswithcode_id,omitempty"`
	MainSize         int64  `json:"mainSize,omitempty"`

	TaskCategories      []string `json:"taskCategories,omitempty"`
	TaskIDs             []string `json:"taskIds,omitempty"`
	Languages           []string `json:"language,omitempty"`
	Multilinguality     []string `json:"multilinguality,omitempty"`
	SizeCategories      []string `json:"sizeCategories,omitempty"`
	SourceDatasets      []string `json:"sourceDatasets,omitempty"`
	AnnotationsCreators []string `json:"annotationsCreators,omitempty"`
	LanguageCreators    []string `json:"languageCreators,omitempty"`
	PrettyName          string   `json:"prettyName,omitempty"`

	DatasetInfo json.RawMessage `json:"datasetInfo,omitempty"`
	Configs     []DatasetConfig `json:"configs,omitempty"`

	// ServerInfo is the dataset-viewer summary the hub attaches to a dataset row:
	// row count, formats, modalities, and whether the viewer works at all. It is
	// what the viewer commands would otherwise cost a request to learn.
	ServerInfo *DatasetServerInfo `json:"datasetsServerInfo,omitempty"`

	// Page-derived.
	HasParquetFormat bool          `json:"hasParquetFormat,omitempty"`
	Libraries        []DatasetLib  `json:"libraries,omitempty"`
	LinkedSpaces     []LinkedSpace `json:"linkedSpaces,omitempty"`
	ViewerEnabled    bool          `json:"viewerEnabled,omitempty"`
	IsTracesDataset  bool          `json:"isTracesDataset,omitempty"`
	IsBenchmark      bool          `json:"isBenchmark,omitempty"`
	IsTraces         bool          `json:"isTraces,omitempty"`

	// ViewerData is the first page of rows plus the schema, exactly as the page
	// rendered it. It is one request where the viewer API would be three, so it
	// is kept whole rather than reshaped.
	ViewerData json.RawMessage `json:"viewerData,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra. The
// dropped key is a sort key the hub sends empty on every dataset row.
func (d *Dataset) UnmarshalJSON(b []byte) error {
	type raw Dataset
	return decodeExtra(b, (*raw)(d), &d.Extra, "key")
}

// DatasetServerInfo is what the dataset viewer knows about a dataset without
// being asked: whether it can render it, how many rows there are, and which
// libraries can load it.
type DatasetServerInfo struct {
	Viewer     string   `json:"viewer,omitempty"`
	NumRows    int64    `json:"numRows,omitempty"`
	Libraries  []string `json:"libraries,omitempty"`
	Formats    []string `json:"formats,omitempty"`
	Modalities []string `json:"modalities,omitempty"`

	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON sweeps the unmodelled viewer keys into Extra.
func (i *DatasetServerInfo) UnmarshalJSON(b []byte) error {
	type raw DatasetServerInfo
	return decodeExtra(b, (*raw)(i), &i.Extra)
}

// DatasetConfig is one named configuration and the files it draws from.
type DatasetConfig struct {
	Name      string         `json:"config_name"`
	DataFiles []DataFileSpec `json:"data_files,omitempty"`
}

// DataFileSpec maps a split to the files that make it up.
type DataFileSpec struct {
	Split string     `json:"split"`
	Path  StringList `json:"path"`
}

// DatasetLib is the loading snippet the page shows per library.
type DatasetLib struct {
	Library string `json:"library"`
	Code    string `json:"code,omitempty"`
	Loading string `json:"loading,omitempty"`
}

// Space is an application repository.
type Space struct {
	Repo

	SDK       string   `json:"sdk,omitempty"`
	Subdomain string   `json:"subdomain,omitempty"`
	Models    []string `json:"models,omitempty"`
	Datasets  []string `json:"datasets,omitempty"`
	Runtime   *Runtime `json:"runtime,omitempty"`

	// Page-derived.
	IframeSrc          string `json:"iframeSrc,omitempty"`
	ShortDescription   string `json:"shortDescription,omitempty"`
	Emoji              string `json:"emoji,omitempty"`
	ColorFrom          string `json:"colorFrom,omitempty"`
	ColorTo            string `json:"colorTo,omitempty"`
	Pinned             bool   `json:"pinned,omitempty"`
	ShowGettingStarted bool   `json:"showGettingStarted,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (s *Space) UnmarshalJSON(b []byte) error {
	type raw Space
	return decodeExtra(b, (*raw)(s), &s.Extra)
}

// Runtime is the live state of a space, which is the one thing about a space
// that no static record can tell you.
type Runtime struct {
	Stage        string          `json:"stage"`
	Hardware     *Hardware       `json:"hardware,omitempty"`
	GCTimeout    int             `json:"gcTimeout,omitempty"`
	Replicas     *Replicas       `json:"replicas,omitempty"`
	DevMode      bool            `json:"devMode,omitempty"`
	Domains      []SpaceDomain   `json:"domains,omitempty"`
	SHA          string          `json:"sha,omitempty"`
	ErrorMessage string          `json:"errorMessage,omitempty"`
	Resources    json.RawMessage `json:"resources,omitempty"`
}

// Hardware is the accelerator a space runs on, current and requested.
type Hardware struct {
	Current   string `json:"current,omitempty"`
	Requested string `json:"requested,omitempty"`
}

// Replicas is the scaling state. Requested stays raw because upstream sends an
// integer or the string "auto" in the same field, and inventing a union type
// for one field is worse than showing the caller what arrived.
type Replicas struct {
	Current   int             `json:"current,omitempty"`
	Requested json.RawMessage `json:"requested,omitempty"`
}

// SpaceDomain is one hostname a space answers on.
type SpaceDomain struct {
	Domain   string `json:"domain"`
	Stage    string `json:"stage,omitempty"`
	IsCustom bool   `json:"isCustom,omitempty"`
}

// Kernel is a compute kernel repository. It is the newest repo kind and the
// thinnest, and it is here because namespace counts include it and leaving it
// out would make an org's totals not add up.
type Kernel struct {
	Repo

	SupportedDriverFamilies []string `json:"supportedDriverFamilies,omitempty"`
	Variants                []string `json:"variants,omitempty"`

	// TrustedPublisher says the build artefacts came from a verified pipeline
	// rather than an upload, which for a binary you are about to run in your own
	// process is the field that matters most on the record.
	TrustedPublisher bool `json:"trustedPublisher,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (k *Kernel) UnmarshalJSON(b []byte) error {
	type raw Kernel
	return decodeExtra(b, (*raw)(k), &k.Extra)
}

// marshalWithExtra writes v, then folds the unknown keys back in at the top
// level so a round trip does not nest them under "extra" for the types that
// stand in for someone else's document.
func marshalWithExtra(v any, extra map[string]json.RawMessage) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return b, nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return b, nil //nolint:nilerr
	}
	for k, raw := range extra {
		if _, taken := m[k]; !taken {
			m[k] = raw
		}
	}
	return json.Marshal(m)
}
