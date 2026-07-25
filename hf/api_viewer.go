package hf

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"

	"github.com/tamnd/any-cli/kit/errs"
)

// api_viewer.go talks to the dataset viewer, which is a separate host with its
// own contract. Everything here is about what is inside a dataset rather than
// what its card says about it.

func (c *Client) viewer(path string, kv ...string) string {
	return query(c.Viewer+"/"+path, kv...)
}

// IsValid is the gate. A dataset with Viewer false has no rows to read, and the
// row commands say so rather than retrying.
func (c *Client) IsValid(ctx context.Context, dataset string) (*Validity, error) {
	var v Validity
	resp, err := c.GetJSON(ctx, c.viewer("is-valid", "dataset", dataset), &v)
	if err != nil {
		return nil, err
	}
	v.normalize(dataset, resp.URL)
	return &v, nil
}

// Splits lists a dataset's config and split pairs. The pending and failed lists
// are real and are surfaced as a status on the record, because a dataset can be
// half processed and silently dropping those would look like missing data.
func (c *Client) Splits(ctx context.Context, dataset string) ([]Split, error) {
	u := c.viewer("splits", "dataset", dataset)
	var body struct {
		Splits  []Split `json:"splits"`
		Pending []Split `json:"pending"`
		Failed  []Split `json:"failed"`
	}
	resp, err := c.GetJSON(ctx, u, &body)
	if err != nil {
		return nil, err
	}
	out := make([]Split, 0, len(body.Splits)+len(body.Pending)+len(body.Failed))
	for _, group := range []struct {
		status string
		splits []Split
	}{
		{"ready", body.Splits},
		{"pending", body.Pending},
		{"failed", body.Failed},
	} {
		for _, s := range group.splits {
			if s.Status == "" {
				s.Status = group.status
			}
			if s.Dataset == "" {
				s.Dataset = dataset
			}
			s.normalize(resp.URL)
			out = append(out, s)
		}
	}
	return out, nil
}

// Sizes reports a dataset's size at all three levels. The response nests them,
// and this flattens it to one record per level so the stream stays flat.
func (c *Client) Sizes(ctx context.Context, dataset string) ([]Size, error) {
	u := c.viewer("size", "dataset", dataset)
	var body struct {
		Size struct {
			Dataset Size   `json:"dataset"`
			Configs []Size `json:"configs"`
			Splits  []Size `json:"splits"`
		} `json:"size"`
	}
	resp, err := c.GetJSON(ctx, u, &body)
	if err != nil {
		return nil, err
	}
	out := make([]Size, 0, 1+len(body.Size.Configs)+len(body.Size.Splits))
	d := body.Size.Dataset
	d.normalize("dataset", resp.URL)
	out = append(out, d)
	for _, s := range body.Size.Configs {
		s.normalize("config", resp.URL)
		out = append(out, s)
	}
	for _, s := range body.Size.Splits {
		s.normalize("split", resp.URL)
		out = append(out, s)
	}
	return out, nil
}

// rowPage is the envelope /rows, /first-rows, /search, and /filter all share.
type rowPage struct {
	Features     []Feature         `json:"features"`
	Rows         []json.RawMessage `json:"rows"`
	NumRowsTotal int64             `json:"num_rows_total"`
	NumRowsPage  int               `json:"num_rows_per_page"`
	Partial      bool              `json:"partial"`
}

// RowOptions selects which rows to read.
type RowOptions struct {
	Config string
	Split  string
	Offset int64
	Limit  int
	// Query runs a full text search, Where runs a SQL-ish predicate. Both need
	// the matching capability on the is-valid record.
	Query   string
	Where   string
	OrderBy string
}

// Features returns a split's schema without reading any rows.
func (c *Client) Features(ctx context.Context, dataset string, o RowOptions) ([]Feature, error) {
	u := c.viewer("first-rows", "dataset", dataset, "config", o.Config, "split", o.Split)
	var page rowPage
	if _, err := c.GetJSON(ctx, u, &page); err != nil {
		return nil, err
	}
	for i := range page.Features {
		page.Features[i].normalize()
	}
	return page.Features, nil
}

// Rows streams a split's rows, paging by 100 because that is the server's cap.
// The features of the first page are handed to onSchema, once, so a caller can
// write a header before the rows arrive.
func (c *Client) Rows(ctx context.Context, dataset string, o RowOptions, onSchema func([]Feature), emit func(*Row) error) error {
	if o.Split == "" {
		return errs.Usage("rows needs a split")
	}
	route := "rows"
	switch {
	case o.Query != "":
		route = "search"
	case o.Where != "":
		route = "filter"
	}

	const pageSize = 100
	offset := o.Offset
	sent := 0
	first := true
	for {
		want := pageSize
		if o.Limit > 0 && o.Limit-sent < want {
			want = o.Limit - sent
		}
		if want <= 0 {
			return nil
		}
		u := c.viewer(route,
			"dataset", dataset,
			"config", o.Config,
			"split", o.Split,
			"offset", strconv.FormatInt(offset, 10),
			"length", strconv.Itoa(want),
			"query", o.Query,
			"where", o.Where,
			"orderby", o.OrderBy,
		)
		var page rowPage
		resp, err := c.GetJSON(ctx, u, &page)
		if err != nil {
			return err
		}
		if first {
			for i := range page.Features {
				page.Features[i].normalize()
			}
			if onSchema != nil {
				onSchema(page.Features)
			}
			first = false
		}
		if len(page.Rows) == 0 {
			return nil
		}
		for _, raw := range page.Rows {
			var r Row
			if err := jsonUnmarshal(raw, &r); err != nil {
				return err
			}
			r.normalize(dataset, o.Config, o.Split, resp.URL)
			if err := emit(&r); err != nil {
				return err
			}
			sent++
			offset++
			if o.Limit > 0 && sent >= o.Limit {
				return nil
			}
		}
		if page.NumRowsTotal > 0 && offset >= page.NumRowsTotal {
			return nil
		}
	}
}

// Statistics returns per-column statistics for a split.
func (c *Client) Statistics(ctx context.Context, dataset string, o RowOptions) ([]ColumnStats, error) {
	u := c.viewer("statistics", "dataset", dataset, "config", o.Config, "split", o.Split)
	var body struct {
		NumExamples int64         `json:"num_examples"`
		Statistics  []ColumnStats `json:"statistics"`
		Partial     bool          `json:"partial"`
	}
	resp, err := c.GetJSON(ctx, u, &body)
	if err != nil {
		return nil, err
	}
	for i := range body.Statistics {
		body.Statistics[i].normalize(dataset, o.Config, o.Split, resp.URL)
	}
	return body.Statistics, nil
}

// Parquet lists a dataset's converted shards. This route lives on the main host
// rather than the viewer host, and it answers a nested map of config to split
// to URL list, which is flattened to one record per shard.
func (c *Client) Parquet(ctx context.Context, dataset string) ([]ParquetShard, error) {
	u := c.api("datasets", dataset, "parquet")
	resp, err := c.Get(ctx, u)
	if err != nil {
		return nil, err
	}
	var byConfig map[string]map[string][]json.RawMessage
	if err := jsonUnmarshal(resp.Body, &byConfig); err != nil {
		return nil, errs.Wrap(errs.KindNetwork, err, "decode %s", shortURL(u))
	}
	var out []ParquetShard
	configs := sortedKeys(byConfig)
	for _, config := range configs {
		splits := sortedKeys(byConfig[config])
		for _, split := range splits {
			for i, raw := range byConfig[config][split] {
				sh := ParquetShard{Dataset: dataset, Config: config, Split: split, Index: i}
				// Older responses are a bare URL string, newer ones an object.
				var s string
				if jsonUnmarshal(raw, &s) == nil {
					sh.URL = s
				} else if err := jsonUnmarshal(raw, &sh); err != nil {
					return nil, err
				}
				sh.Dataset, sh.Config, sh.Split, sh.Index = dataset, config, split, i
				sh.normalize(resp.URL)
				out = append(out, sh)
			}
		}
	}
	return out, nil
}

// Croissant returns the dataset's MLCommons Croissant document untouched. It is
// already JSON-LD over schema.org, which makes it the strongest linked-data
// artifact on the hub, and reserialising it here would only lose detail.
func (c *Client) Croissant(ctx context.Context, dataset string) (json.RawMessage, error) {
	resp, err := c.Get(ctx, c.api("datasets", dataset, "croissant"))
	if err != nil {
		return nil, err
	}
	return json.RawMessage(resp.Body), nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
