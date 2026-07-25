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

	Dataset    string `json:"dataset" table:"dataset"`
	Preview    bool   `json:"preview" table:"preview"`
	Viewer     bool   `json:"viewer" table:"viewer"`
	Search     bool   `json:"search" table:"search"`
	Filter     bool   `json:"filter" table:"filter"`
	Statistics bool   `json:"statistics" table:"statistics"`
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

	Dataset string          `json:"dataset" table:"dataset"`
	Config  string          `json:"config" table:"config"`
	Split   string          `json:"split" table:"split"`
	Status  string          `json:"status,omitempty" table:"status"`
	Error   json.RawMessage `json:"error,omitempty" table:"-"`
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

	Level   string `json:"level" table:"level"`
	Dataset string `json:"dataset" table:"-"`
	Config  string `json:"config,omitempty" table:"config"`
	Split   string `json:"split,omitempty" table:"split"`

	NumBytesOriginalFiles int64  `json:"num_bytes_original_files,omitempty" table:"-"`
	NumBytesParquetFiles  int64  `json:"num_bytes_parquet_files,omitempty" table:"parquet_bytes"`
	NumBytesMemory        int64  `json:"num_bytes_memory,omitempty" table:"-"`
	NumRows               int64  `json:"num_rows,omitempty" table:"rows"`
	NumColumns            int    `json:"num_columns,omitempty" table:"-"`
	EstimatedNumRows      *int64 `json:"estimated_num_rows,omitempty" table:"-"`
	Partial               bool   `json:"partial,omitempty" table:"-"`
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

	Dataset        string                     `json:"dataset" table:"-"`
	Config         string                     `json:"config" table:"-"`
	Split          string                     `json:"split" table:"-"`
	Index          int64                      `json:"row_idx" table:"row_idx"`
	Row            map[string]json.RawMessage `json:"row" table:"row,truncate"`
	TruncatedCells []string                   `json:"truncated_cells,omitempty" table:"-"`
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

	Dataset string `json:"dataset" table:"-"`
	Config  string `json:"config" table:"-"`
	Split   string `json:"split" table:"-"`

	Column string `json:"column_name" table:"column"`
	Type   string `json:"column_type" table:"type"`

	NaNCount      int64            `json:"nan_count,omitempty" table:"nan"`
	NaNProportion float64          `json:"nan_proportion,omitempty" table:"-"`
	Min           *float64         `json:"min,omitempty" table:"-"`
	Max           *float64         `json:"max,omitempty" table:"-"`
	Mean          *float64         `json:"mean,omitempty" table:"-"`
	Median        *float64         `json:"median,omitempty" table:"-"`
	Std           *float64         `json:"std,omitempty" table:"-"`
	Histogram     *Histogram       `json:"histogram,omitempty" table:"-"`
	Frequencies   map[string]int64 `json:"frequencies,omitempty" table:"-"`
	NoLabelCount  int64            `json:"no_label_count,omitempty" table:"-"`
	NumDistinct   int64            `json:"n_unique,omitempty" table:"distinct"`
	MinLength     *int64           `json:"min_length,omitempty" table:"-"`
	MaxLength     *int64           `json:"max_length,omitempty" table:"-"`
	Raw           json.RawMessage  `json:"raw,omitempty" table:"-"`
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

	Dataset  string `json:"dataset" table:"-"`
	Config   string `json:"config" table:"config"`
	Split    string `json:"split" table:"split"`
	Index    int    `json:"index" table:"index"`
	Filename string `json:"filename,omitempty" table:"filename"`
	Size     int64  `json:"size,omitempty" table:"size"`
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
