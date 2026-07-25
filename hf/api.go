package hf

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// api.go holds what every fetch method shares: the route vocabulary, the expand
// sets, and the generic paginator.

// apiPath maps a repo kind to its API path segment. The pluralisation is the
// hub's, and it does not match the URL a human sees, which is why this table
// exists rather than a string concatenation at each call site.
var apiPath = map[string]string{
	KindModel:   "models",
	KindDataset: "datasets",
	KindSpace:   "spaces",
	KindKernel:  "kernels",
}

// repoAPIPath validates a repo kind and returns its API segment.
func repoAPIPath(kind string) (string, error) {
	p, ok := apiPath[kind]
	if !ok {
		return "", errs.Usage("%q is not a repository kind, want one of model, dataset, space, kernel", kind)
	}
	return p, nil
}

// The expand vocabularies, taken verbatim from the server's own 400 messages.
// They are exact enums and the server rejects the whole request over one wrong
// name, so this table is transcription rather than judgement.
//
// usedStorage is the field every kind accepts on its detail route and rejects
// on its list route, which is why detailOnly exists.
var (
	modelExpand = []string{
		"author", "baseModels", "cardData", "config", "createdAt", "disabled",
		"downloads", "downloadsAllTime", "evalResults", "gated", "inference",
		"inferenceProviderMapping", "lastModified", "library_name", "likes",
		"mask_token", "model-index", "pipeline_tag", "private", "safetensors",
		"sha", "siblings", "spaces", "tags", "transformersInfo", "trendingScore",
		"widgetData", "gguf", "resourceGroup", "xetEnabled",
	}

	datasetExpand = []string{
		"author", "cardData", "citation", "createdAt", "disabled", "description",
		"downloads", "downloadsAllTime", "gated", "lastModified", "likes",
		"mainSize", "paperswithcode_id", "private", "siblings", "sha", "tags",
		"trendingScore", "resourceGroup", "xetEnabled",
	}

	// Spaces have no gated, downloads, or sdk-independent size, and they do have
	// runtime and subdomain, which no other kind has.
	spaceExpand = []string{
		"author", "cardData", "createdAt", "datasets", "disabled", "lastModified",
		"likes", "models", "private", "runtime", "sdk", "sha", "siblings",
		"subdomain", "tags", "trendingScore", "resourceGroup", "xetEnabled",
	}

	kernelExpand = []string{
		"author", "cardData", "createdAt", "downloads", "downloadsAllTime",
		"gated", "lastModified", "likes", "private", "sha", "siblings", "tags",
		"trendingScore",
	}

	// detailOnly is what each kind's detail route accepts on top of its list set.
	detailOnly = map[string][]string{
		KindModel:   {"childrenModelCount", "usedStorage"},
		KindDataset: {"usedStorage"},
		KindSpace:   {"usedStorage"},
	}
)

// expandFor returns the expand set for a kind, plus the fields only the detail
// route accepts.
func expandFor(kind string, detail bool) []string {
	var base []string
	switch kind {
	case KindModel:
		base = modelExpand
	case KindDataset:
		base = datasetExpand
	case KindSpace:
		base = spaceExpand
	case KindKernel:
		base = kernelExpand
	default:
		return nil
	}
	if !detail {
		return base
	}
	extra := detailOnly[kind]
	if len(extra) == 0 {
		return base
	}
	out := make([]string, 0, len(base)+len(extra))
	return append(append(out, base...), extra...)
}

// withExpand appends the repeated expand[] parameters.
func withExpand(rawURL string, fields []string) string {
	if len(fields) == 0 {
		return rawURL
	}
	var b strings.Builder
	b.WriteString(rawURL)
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	for _, f := range fields {
		b.WriteString(sep)
		b.WriteString("expand%5B%5D=")
		b.WriteString(f)
		sep = "&"
	}
	return b.String()
}

// pageLimit clamps the request-side limit to what an endpoint accepts. The
// social endpoints reject anything under 10, so asking for three followers has
// to ask for ten and stop early, which is what the caller's own limit does.
func pageLimit(want, min, max int) string {
	if want <= 0 {
		return strconv.Itoa(max)
	}
	if want < min {
		want = min
	}
	if want > max {
		want = max
	}
	return strconv.Itoa(want)
}

// walk is the generic paginator. decode turns one raw element into a record,
// emit consumes it, and limit stops the walk without requesting another page.
// A limit of zero means keep going until the cursor runs out.
func walk[T any](ctx context.Context, c *Client, rawURL string, limit int,
	decode func(json.RawMessage, *Response) (*T, error),
	emit func(*T) error,
) error {
	n := 0
	return c.eachPage(ctx, rawURL, func(items []json.RawMessage, resp *Response) error {
		for _, raw := range items {
			rec, err := decode(raw, resp)
			if err != nil {
				return err
			}
			if rec == nil {
				continue
			}
			if err := emit(rec); err != nil {
				return err
			}
			n++
			if limit > 0 && n >= limit {
				return errStopPage
			}
		}
		return nil
	})
}

// walkWrapped is walk for the endpoints that wrap their array in an object.
// Discussions and posts both do this, and the key differs per endpoint.
func walkWrapped[T any](ctx context.Context, c *Client, rawURL, key string, limit int,
	decode func(json.RawMessage, *Response) (*T, error),
	emit func(*T) error,
) error {
	n := 0
	for rawURL != "" {
		resp, err := c.Get(ctx, rawURL)
		if err != nil {
			return err
		}
		var env map[string]json.RawMessage
		if err := jsonUnmarshal(resp.Body, &env); err != nil {
			return errs.Wrap(errs.KindNetwork, err, "decode %s", shortURL(rawURL))
		}
		var items []json.RawMessage
		if raw, ok := env[key]; ok {
			if err := jsonUnmarshal(raw, &items); err != nil {
				return errs.Wrap(errs.KindNetwork, err, "decode %s in %s", key, shortURL(rawURL))
			}
		}
		for _, raw := range items {
			rec, err := decode(raw, resp)
			if err != nil {
				return err
			}
			if rec == nil {
				continue
			}
			if err := emit(rec); err != nil {
				return err
			}
			n++
			if limit > 0 && n >= limit {
				return nil
			}
		}
		if len(items) == 0 {
			return nil
		}
		rawURL = nextLink(resp.Header)
	}
	return nil
}

// aliasOf records that the hub redirected a legacy id to its canonical one. A
// tag of dataset:squad and a repo id of rajpurkar/squad have to resolve to one
// node, and this is where that gets noticed.
func aliasOf(requested string, resp *Response) string {
	if resp == nil || resp.FinalURL == "" || resp.FinalURL == resp.URL {
		return ""
	}
	return requested
}
