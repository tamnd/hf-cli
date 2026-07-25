package hf

import (
	"encoding/json"
	"strconv"
	"strings"
)

// viewer.go covers the dataset-viewer service, which is a separate host with a
// separate contract. It is the only way to get at what is actually inside a
// dataset rather than what its card says about it.

// Validity is the viewer's answer to what is possible for a dataset. Check it
// before anything else; a dataset with Viewer false has no rows.
type Validity struct {
	Meta

	Dataset    string `json:"dataset"`
	Preview    bool   `json:"preview"`
	Viewer     bool   `json:"viewer"`
	Search     bool   `json:"search"`
	Filter     bool   `json:"filter"`
	Statistics bool   `json:"statistics"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (v *Validity) UnmarshalJSON(b []byte) error {
	type raw Validity
	return decodeExtra(b, (*raw)(v), &v.Extra)
}

func (v *Validity) normalize(dataset, sourceURL string) {
	v.Dataset = dataset
	v.setMeta(KindDataset, dataset, sourceURL)
}

// Split is one config and split pair of a dataset.
type Split struct {
	Meta

	Dataset string          `json:"dataset"`
	Config  string          `json:"config"`
	Split   string          `json:"split"`
	Status  string          `json:"status,omitempty"`
	Error   json.RawMessage `json:"error,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (s *Split) UnmarshalJSON(b []byte) error {
	type raw Split
	return decodeExtra(b, (*raw)(s), &s.Extra)
}

func (s *Split) normalize(sourceURL string) {
	s.setMeta(KindSplit, SplitID(s.Dataset, s.Config, s.Split), sourceURL)
	s.URL = BaseURL + "/datasets/" + s.Dataset + "/viewer/" + escapePath(s.Config) + "/" + escapePath(s.Split)
}

// Size is emitted once per level, so the stream is flat rather than a nested
// document a consumer would have to walk.
type Size struct {
	Meta

	Level   string `json:"level"`
	Dataset string `json:"dataset"`
	Config  string `json:"config,omitempty"`
	Split   string `json:"split,omitempty"`

	NumBytesOriginalFiles int64  `json:"num_bytes_original_files,omitempty"`
	NumBytesParquetFiles  int64  `json:"num_bytes_parquet_files,omitempty"`
	NumBytesMemory        int64  `json:"num_bytes_memory,omitempty"`
	NumRows               int64  `json:"num_rows,omitempty"`
	NumColumns            int    `json:"num_columns,omitempty"`
	EstimatedNumRows      *int64 `json:"estimated_num_rows,omitempty"`
	Partial               bool   `json:"partial,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (s *Size) UnmarshalJSON(b []byte) error {
	type raw Size
	return decodeExtra(b, (*raw)(s), &s.Extra)
}

func (s *Size) normalize(level, sourceURL string) {
	s.Level = level
	s.setMeta(KindSplit, SplitID(s.Dataset, s.Config, s.Split), sourceURL)
}

// Feature is one column's schema. Type stays raw because the datasets library
// type system is recursive and rich, and re-encoding it here would lose detail.
type Feature struct {
	Index int             `json:"feature_idx"`
	Name  string          `json:"name"`
	Type  json.RawMessage `json:"type,omitempty"`
	DType string          `json:"dtype,omitempty"`
	Class string          `json:"class,omitempty"`
}

// normalize pulls the two things a person actually reads out of the raw type,
// and leaves the rest in place for whoever needs the full shape.
func (f *Feature) normalize() {
	if len(f.Type) == 0 {
		return
	}
	var m map[string]json.RawMessage
	if jsonUnmarshal(f.Type, &m) != nil {
		return
	}
	if v, ok := m["dtype"]; ok {
		var s string
		if jsonUnmarshal(v, &s) == nil {
			f.DType = s
		}
	}
	if v, ok := m["_type"]; ok {
		var s string
		if jsonUnmarshal(v, &s) == nil {
			f.Class = s
		}
	}
	if f.Class == "" && f.DType != "" {
		f.Class = "Value"
	}
}

// Row is one dataset row, emitted one per line so a split streams.
type Row struct {
	Meta

	Dataset        string                     `json:"dataset"`
	Config         string                     `json:"config"`
	Split          string                     `json:"split"`
	Index          int64                      `json:"row_idx"`
	Row            map[string]json.RawMessage `json:"row"`
	TruncatedCells []string                   `json:"truncated_cells,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (r *Row) UnmarshalJSON(b []byte) error {
	type raw Row
	return decodeExtra(b, (*raw)(r), &r.Extra)
}

func (r *Row) normalize(dataset, config, split, sourceURL string) {
	r.Dataset, r.Config, r.Split = dataset, config, split
	r.setMeta(KindSplit, SplitID(dataset, config, split)+"#"+itoa64(r.Index), sourceURL)
}

// ColumnStats is one column's statistics. The shape of the payload depends on
// the column type, so the common numeric fields are typed and the rest stays
// raw. Min, Max, Mean, Median, and Std are pointers because zero is a
// meaningful statistic and absent is a different thing, a distinction that
// matters on a column of all zeros.
type ColumnStats struct {
	Meta

	Dataset string `json:"dataset"`
	Config  string `json:"config"`
	Split   string `json:"split"`

	Column string `json:"column_name"`
	Type   string `json:"column_type"`

	NaNCount      int64            `json:"nan_count,omitempty"`
	NaNProportion float64          `json:"nan_proportion,omitempty"`
	Min           *float64         `json:"min,omitempty"`
	Max           *float64         `json:"max,omitempty"`
	Mean          *float64         `json:"mean,omitempty"`
	Median        *float64         `json:"median,omitempty"`
	Std           *float64         `json:"std,omitempty"`
	Histogram     *Histogram       `json:"histogram,omitempty"`
	Frequencies   map[string]int64 `json:"frequencies,omitempty"`
	NoLabelCount  int64            `json:"no_label_count,omitempty"`
	NumDistinct   int64            `json:"n_unique,omitempty"`
	MinLength     *int64           `json:"min_length,omitempty"`
	MaxLength     *int64           `json:"max_length,omitempty"`
	Raw           json.RawMessage  `json:"raw,omitempty"`
}

// UnmarshalJSON lifts the nested column_statistics object up to the top level.
// Upstream nests it, but a statistic that needs two hops to reach is a
// statistic nobody uses.
func (c *ColumnStats) UnmarshalJSON(b []byte) error {
	type raw ColumnStats
	var head struct {
		Column string          `json:"column_name"`
		Type   string          `json:"column_type"`
		Stats  json.RawMessage `json:"column_statistics"`
	}
	if err := jsonUnmarshal(b, &head); err != nil {
		return err
	}
	if len(head.Stats) > 0 {
		var r raw
		if err := decodeExtra(head.Stats, &r, &c.Extra); err != nil {
			return err
		}
		*c = ColumnStats(r)
		c.Column, c.Type = head.Column, head.Type
		c.Raw = head.Stats
		return nil
	}
	return decodeExtra(b, (*raw)(c), &c.Extra)
}

func (c *ColumnStats) normalize(dataset, config, split, sourceURL string) {
	c.Dataset, c.Config, c.Split = dataset, config, split
	c.setMeta(KindSplit, SplitID(dataset, config, split)+"#"+c.Column, sourceURL)
}

// Histogram is a numeric column's distribution.
type Histogram struct {
	Hist     []int64   `json:"hist"`
	BinEdges []float64 `json:"bin_edges"`
}

// ParquetShard is one converted file. These are the bytes to fetch when the row
// API is too slow, and the hub converts every viewable dataset to them.
type ParquetShard struct {
	Meta

	Dataset  string `json:"dataset"`
	Config   string `json:"config"`
	Split    string `json:"split"`
	Index    int    `json:"index"`
	Filename string `json:"filename,omitempty"`
	Size     int64  `json:"size,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra. The
// viewer names the download link url, which collides with Meta.URL, so it is
// mapped explicitly.
func (p *ParquetShard) UnmarshalJSON(b []byte) error {
	type raw ParquetShard
	var r raw
	if err := decodeExtra(b, &r, &p.Extra); err != nil {
		return err
	}
	*p = ParquetShard(r)
	var head struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(b, &head) == nil && head.URL != "" {
		p.URL = head.URL
	}
	delete(p.Extra, "url")
	return nil
}

func (p *ParquetShard) normalize(sourceURL string) {
	if p.Filename == "" && p.URL != "" {
		if i := strings.LastIndexByte(p.URL, '/'); i >= 0 {
			p.Filename = p.URL[i+1:]
		}
	}
	url := p.URL
	p.setMeta(KindSplit, SplitID(p.Dataset, p.Config, p.Split)+"/"+p.Filename, sourceURL)
	p.URL = url
}

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
