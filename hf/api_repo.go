package hf

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// api_repo.go fetches repositories and everything inside them.

// ListOptions is the shared query surface of the four list endpoints. Only
// Filter actually filters: the hub accepts other= and dataset= and ignores
// them, which is why neither appears here.
type ListOptions struct {
	Search    string
	Author    string
	Filter    []string
	Sort      string
	Direction string
	Limit     int
	Expand    bool
}

func (o ListOptions) url(c *Client, kind string) (string, error) {
	path, err := repoAPIPath(kind)
	if err != nil {
		return "", err
	}
	u := query(c.api(path),
		"search", o.Search,
		"author", o.Author,
		"sort", o.Sort,
		"direction", o.Direction,
		"limit", pageLimit(o.Limit, 1, 1000),
	)
	for _, f := range o.Filter {
		u = query(u, "filter", f)
	}
	if o.Expand {
		u = withExpand(u, expandFor(kind, false))
	}
	return u, nil
}

// Models walks the model list, calling emit once per model.
func (c *Client) Models(ctx context.Context, o ListOptions, emit func(*Model) error) error {
	u, err := o.url(c, KindModel)
	if err != nil {
		return err
	}
	return walk(ctx, c, u, o.Limit, func(raw json.RawMessage, resp *Response) (*Model, error) {
		var m Model
		if err := jsonUnmarshal(raw, &m); err != nil {
			return nil, err
		}
		m.normalize(KindModel, resp.URL)
		return &m, nil
	}, emit)
}

// Datasets walks the dataset list.
func (c *Client) Datasets(ctx context.Context, o ListOptions, emit func(*Dataset) error) error {
	u, err := o.url(c, KindDataset)
	if err != nil {
		return err
	}
	return walk(ctx, c, u, o.Limit, func(raw json.RawMessage, resp *Response) (*Dataset, error) {
		var d Dataset
		if err := jsonUnmarshal(raw, &d); err != nil {
			return nil, err
		}
		d.normalize(KindDataset, resp.URL)
		return &d, nil
	}, emit)
}

// Spaces walks the space list.
func (c *Client) Spaces(ctx context.Context, o ListOptions, emit func(*Space) error) error {
	u, err := o.url(c, KindSpace)
	if err != nil {
		return err
	}
	return walk(ctx, c, u, o.Limit, func(raw json.RawMessage, resp *Response) (*Space, error) {
		var s Space
		if err := jsonUnmarshal(raw, &s); err != nil {
			return nil, err
		}
		s.normalize(KindSpace, resp.URL)
		return &s, nil
	}, emit)
}

// Kernels walks the kernel list.
func (c *Client) Kernels(ctx context.Context, o ListOptions, emit func(*Kernel) error) error {
	u, err := o.url(c, KindKernel)
	if err != nil {
		return err
	}
	return walk(ctx, c, u, o.Limit, func(raw json.RawMessage, resp *Response) (*Kernel, error) {
		var k Kernel
		if err := jsonUnmarshal(raw, &k); err != nil {
			return nil, err
		}
		k.normalize(KindKernel, resp.URL)
		return &k, nil
	}, emit)
}

// repoDetailURL builds a detail route, optionally pinned to a revision.
func (c *Client) repoDetailURL(kind, id, rev string) (string, error) {
	path, err := repoAPIPath(kind)
	if err != nil {
		return "", err
	}
	u := c.api(path, id)
	if rev != "" {
		u = c.api(path, id, "revision", escapeRev(rev))
	}
	return withExpand(u, expandFor(kind, true)), nil
}

// Model fetches one model with every expand field the detail route accepts.
func (c *Client) Model(ctx context.Context, id, rev string) (*Model, error) {
	u, err := c.repoDetailURL(KindModel, id, rev)
	if err != nil {
		return nil, err
	}
	var m Model
	resp, err := c.GetJSON(ctx, u, &m)
	if err != nil {
		return nil, err
	}
	m.normalize(KindModel, resp.URL)
	m.AliasOf = aliasOf(URI(KindModel, id), resp)
	if err := c.enrichRepo(ctx, KindModel, &m.Repo, rev); err != nil {
		return nil, err
	}
	return &m, nil
}

// Dataset fetches one dataset.
func (c *Client) Dataset(ctx context.Context, id, rev string) (*Dataset, error) {
	u, err := c.repoDetailURL(KindDataset, id, rev)
	if err != nil {
		return nil, err
	}
	var d Dataset
	resp, err := c.GetJSON(ctx, u, &d)
	if err != nil {
		return nil, err
	}
	d.normalize(KindDataset, resp.URL)
	d.AliasOf = aliasOf(URI(KindDataset, id), resp)
	if err := c.enrichRepo(ctx, KindDataset, &d.Repo, rev); err != nil {
		return nil, err
	}
	return &d, nil
}

// Space fetches one space.
func (c *Client) Space(ctx context.Context, id, rev string) (*Space, error) {
	u, err := c.repoDetailURL(KindSpace, id, rev)
	if err != nil {
		return nil, err
	}
	var s Space
	resp, err := c.GetJSON(ctx, u, &s)
	if err != nil {
		return nil, err
	}
	s.normalize(KindSpace, resp.URL)
	s.AliasOf = aliasOf(URI(KindSpace, id), resp)
	if err := c.enrichRepo(ctx, KindSpace, &s.Repo, rev); err != nil {
		return nil, err
	}
	return &s, nil
}

// Kernel fetches one kernel.
func (c *Client) Kernel(ctx context.Context, id, rev string) (*Kernel, error) {
	u, err := c.repoDetailURL(KindKernel, id, rev)
	if err != nil {
		return nil, err
	}
	var k Kernel
	resp, err := c.GetJSON(ctx, u, &k)
	if err != nil {
		return nil, err
	}
	k.normalize(KindKernel, resp.URL)
	k.AliasOf = aliasOf(URI(KindKernel, id), resp)
	if err := c.enrichRepo(ctx, KindKernel, &k.Repo, rev); err != nil {
		return nil, err
	}
	return &k, nil
}

// RepoRecord fetches whichever repo kind is asked for, as the concrete type.
// The commands that take a kind flag go through here so the four cases live in
// one place rather than in every caller.
func (c *Client) RepoRecord(ctx context.Context, kind, id, rev string) (any, error) {
	switch kind {
	case KindModel:
		return c.Model(ctx, id, rev)
	case KindDataset:
		return c.Dataset(ctx, id, rev)
	case KindSpace:
		return c.Space(ctx, id, rev)
	case KindKernel:
		return c.Kernel(ctx, id, rev)
	default:
		return nil, errs.Usage("%q is not a repository kind", kind)
	}
}

// enrichRepo adds what the API route does not carry: the README under --card,
// and the page-only fields under --deep. Both are opt-in because both cost a
// second request per repo.
func (c *Client) enrichRepo(ctx context.Context, kind string, r *Repo, rev string) error {
	if !c.Card || r == nil {
		return nil
	}
	if r.CardText != "" {
		return nil
	}
	text, err := c.Readme(ctx, kind, r.ID, rev)
	if err != nil {
		// A repo with no README is normal, and a gated one is the caller's
		// problem to solve with a token, not a reason to lose the record.
		if IsNotFound(err) || IsNeedAuth(err) {
			return nil
		}
		return err
	}
	card, body, perr := ParseReadme(text)
	r.CardText = body
	r.CardExists = true
	if perr != nil {
		r.CardError = perr.Error()
		return nil
	}
	if card != nil && r.CardData == nil {
		r.CardData = card
	}
	return nil
}

// Readme fetches a repo's README.md, which is its card.
func (c *Client) Readme(ctx context.Context, kind, id, rev string) (string, error) {
	resp, err := c.File(ctx, kind, id, rev, "README.md", true)
	if err != nil {
		return "", err
	}
	return string(resp.Body), nil
}

// File fetches one file's bytes. pointer picks the raw route, which returns the
// git blob and therefore the LFS pointer text rather than the payload.
func (c *Client) File(ctx context.Context, kind, id, rev, path string, pointer bool) (*Response, error) {
	base, err := repoURL(kind, id)
	if err != nil {
		return nil, err
	}
	route := "/resolve/"
	if pointer {
		route = "/raw/"
	}
	return c.Get(ctx, base+route+escapeRev(rev)+"/"+escapePath(path))
}

// Download copies one file to a writer without buffering it. hf cat uses it, so
// piping a 5GB shard through the tool costs no memory and writes nothing to the
// cache. It returns the number of bytes copied.
func (c *Client) Download(ctx context.Context, kind, id, rev, path string, pointer bool, w io.Writer) (int64, error) {
	base, err := repoURL(kind, id)
	if err != nil {
		return 0, err
	}
	route := "/resolve/"
	if pointer {
		route = "/raw/"
	}
	body, _, err := c.Stream(ctx, base+route+escapeRev(rev)+"/"+escapePath(path))
	if err != nil {
		return 0, err
	}
	defer func() { _ = body.Close() }()
	return io.Copy(w, body)
}

// Refs lists the branches, tags, and conversion refs of a repo. The conversion
// refs matter: refs/convert/parquet is how a dataset's parquet mirror is
// addressed.
func (c *Client) Refs(ctx context.Context, kind, id string) ([]Ref, error) {
	path, err := repoAPIPath(kind)
	if err != nil {
		return nil, err
	}
	u := c.api(path, id, "refs")
	var body struct {
		Branches []Ref `json:"branches"`
		Tags     []Ref `json:"tags"`
		Converts []Ref `json:"converts"`
	}
	resp, err := c.GetJSON(ctx, u, &body)
	if err != nil {
		return nil, err
	}
	out := make([]Ref, 0, len(body.Branches)+len(body.Tags)+len(body.Converts))
	for _, group := range []struct {
		kind string
		refs []Ref
	}{
		{"branch", body.Branches},
		{"tag", body.Tags},
		{"convert", body.Converts},
	} {
		for _, r := range group.refs {
			r.normalize(kind, id, group.kind, resp.URL)
			out = append(out, r)
		}
	}
	return out, nil
}

// TreeOptions controls a tree walk.
type TreeOptions struct {
	Revision  string
	Path      string
	Recursive bool
	// Commits turns on expand=true, which adds lastCommit and the security scan
	// per entry. It joins the commit log per entry and is much slower on a large
	// tree, so it is off by default.
	Commits bool
	Limit   int
}

// Tree walks a repo's files.
func (c *Client) Tree(ctx context.Context, kind, id string, o TreeOptions, emit func(*TreeEntry) error) error {
	path, err := repoAPIPath(kind)
	if err != nil {
		return err
	}
	rev := o.Revision
	if rev == "" {
		rev = "main"
	}
	u := c.api(path, id, "tree", escapeRev(rev))
	if o.Path != "" {
		u += "/" + escapePath(strings.Trim(o.Path, "/"))
	}
	u = query(u, "limit", pageLimit(o.Limit, 1, 1000))
	if o.Recursive {
		u = query(u, "recursive", "true")
	}
	if o.Commits {
		u = query(u, "expand", "true")
	}
	return walk(ctx, c, u, o.Limit, func(raw json.RawMessage, resp *Response) (*TreeEntry, error) {
		var e TreeEntry
		if err := jsonUnmarshal(raw, &e); err != nil {
			return nil, err
		}
		e.normalize(kind, id, rev, resp.URL)
		return &e, nil
	}, emit)
}

// PathsInfo is the one non-GET read in the tool. It exists because it is the
// only way to get the security scan and the LFS pointer for a named set of
// paths without walking the whole tree.
func (c *Client) PathsInfo(ctx context.Context, kind, id, rev string, paths []string) ([]TreeEntry, error) {
	apiSeg, err := repoAPIPath(kind)
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errs.Usage("paths-info needs at least one path")
	}
	if rev == "" {
		rev = "main"
	}
	u := c.api(apiSeg, id, "paths-info", escapeRev(rev))
	resp, err := c.PostJSON(ctx, u, map[string]any{"paths": paths, "expand": true})
	if err != nil {
		return nil, err
	}
	var out []TreeEntry
	if err := jsonUnmarshal(resp.Body, &out); err != nil {
		return nil, errs.Wrap(errs.KindNetwork, err, "decode %s", shortURL(u))
	}
	for i := range out {
		out[i].normalize(kind, id, rev, resp.URL)
	}
	return out, nil
}

// Commits walks a repo's commit log. The authors array on each commit is a user
// edge, and it is a good source of contributor graphs.
func (c *Client) Commits(ctx context.Context, kind, id, rev string, limit int, emit func(*Commit) error) error {
	path, err := repoAPIPath(kind)
	if err != nil {
		return err
	}
	if rev == "" {
		rev = "main"
	}
	u := query(c.api(path, id, "commits", escapeRev(rev)), "limit", pageLimit(limit, 1, 1000))
	return walk(ctx, c, u, limit, func(raw json.RawMessage, resp *Response) (*Commit, error) {
		var cm Commit
		if err := jsonUnmarshal(raw, &cm); err != nil {
			return nil, err
		}
		cm.normalize(kind, id, resp.URL)
		return &cm, nil
	}, emit)
}

// Likers walks the users who liked a repo.
func (c *Client) Likers(ctx context.Context, kind, id string, limit int, emit func(*UserRef) error) error {
	path, err := repoAPIPath(kind)
	if err != nil {
		return err
	}
	u := query(c.api(path, id, "likers"), "limit", pageLimit(limit, 1, 1000))
	return walk(ctx, c, u, limit, decodeUserRef, emit)
}

// DiscussionOptions filters a discussion list.
type DiscussionOptions struct {
	Status string // open or closed
	Type   string // discussion or pull_request
	Author string
	Limit  int
}

// Discussions walks a repo's threads. The list is wrapped in an object rather
// than being a bare array, which is why it does not go through the plain walk.
func (c *Client) Discussions(ctx context.Context, kind, id string, o DiscussionOptions, emit func(*Discussion) error) error {
	path, err := repoAPIPath(kind)
	if err != nil {
		return err
	}
	u := query(c.api(path, id, "discussions"),
		"status", o.Status,
		"type", o.Type,
		"author", o.Author,
		"limit", pageLimit(o.Limit, 1, 1000),
	)
	return walkWrapped(ctx, c, u, "discussions", o.Limit, func(raw json.RawMessage, resp *Response) (*Discussion, error) {
		var d Discussion
		if err := jsonUnmarshal(raw, &d); err != nil {
			return nil, err
		}
		d.normalize(kind, id, resp.URL)
		return &d, nil
	}, emit)
}

// Discussion fetches one thread with its full event stream.
func (c *Client) Discussion(ctx context.Context, kind, id string, num int) (*Discussion, error) {
	path, err := repoAPIPath(kind)
	if err != nil {
		return nil, err
	}
	u := c.api(path, id, "discussions", strconv.Itoa(num))
	var d Discussion
	resp, err := c.GetJSON(ctx, u, &d)
	if err != nil {
		return nil, err
	}
	if d.Num == 0 {
		d.Num = num
	}
	d.normalize(kind, id, resp.URL)
	return &d, nil
}

// decodeUserRef is the shared decode for likers, followers, following, and
// members, all of which return the same compact object.
func decodeUserRef(raw json.RawMessage, resp *Response) (*UserRef, error) {
	var u UserRef
	if err := jsonUnmarshal(raw, &u); err != nil {
		return nil, err
	}
	u.normalize()
	return &u, nil
}

// SpaceRuntime fetches a running space's hardware and status, which changes
// minute to minute and so is worth reading separately from the space record.
func (c *Client) SpaceRuntime(ctx context.Context, id string) (*Runtime, error) {
	var rt Runtime
	if _, err := c.GetJSON(ctx, c.api("spaces", id, "runtime"), &rt); err != nil {
		return nil, err
	}
	return &rt, nil
}

// FileMeta reports what the file store says about a file without downloading
// it. The headers carry the resolved commit, the size behind an LFS pointer,
// and the Xet hash, none of which are in the tree entry.
func (c *Client) FileMeta(ctx context.Context, kind, id, rev, path string) (http.Header, error) {
	resp, err := c.File(ctx, kind, id, rev, path, false)
	if err != nil {
		return nil, err
	}
	return resp.Header, nil
}
