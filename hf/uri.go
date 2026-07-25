package hf

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/tamnd/any-cli/kit/errs"
)

// uri.go is the naming layer. If two parts of the tool name the same thing
// differently the graph is wrong no matter how good the extraction is, so every
// id in the codebase passes through here.
//
// Classify and Locate are pure: no network, no state. That is what lets
// `hf uri` and `hf url` work offline and instantly, and it is the seam a
// multi-domain host uses to route hf:// addresses.

// URI builds the canonical address for an entity: hf://model/owner/name.
func URI(kind, id string) string {
	if kind == "" || id == "" {
		return ""
	}
	return Scheme + "://" + kind + "/" + strings.Trim(id, "/")
}

// SplitURI takes an hf:// address apart. ok is false for anything that is not
// one, so callers can use it as a test.
func SplitURI(s string) (kind, id string, ok bool) {
	rest, found := strings.CutPrefix(s, Scheme+"://")
	if !found {
		return "", "", false
	}
	kind, id, found = strings.Cut(rest, "/")
	if !found || kind == "" || id == "" {
		return "", "", false
	}
	return kind, id, true
}

// hostAliases are the hostnames that mean huggingface.co.
var hostAliases = map[string]bool{
	"huggingface.co":     true,
	"www.huggingface.co": true,
	"hf.co":              true,
	"www.hf.co":          true,
}

// pathKind maps the first path segment of a hub URL to the kind it introduces.
// A path with none of these prefixes is a repo under the model namespace, which
// is why models have no prefix of their own.
var pathKind = map[string]string{
	"datasets":    KindDataset,
	"spaces":      KindSpace,
	"kernels":     KindKernel,
	"collections": KindCollection,
	"papers":      KindPaper,
	"blog":        KindBlog,
	"posts":       KindPost,
	"tasks":       KindTask,
}

// reservedPaths are first segments that are site chrome rather than a
// namespace, so a bare one of them is not a user.
var reservedPaths = map[string]bool{
	"docs": true, "pricing": true, "join": true, "login": true, "settings": true,
	"api": true, "models": true, "search": true, "chat": true, "learn": true,
	"enterprise": true, "brand": true, "terms-of-service": true, "privacy": true,
	"inference-endpoints": true, "changelog": true, "new": true, "notifications": true,
}

// Classify turns any input a person might paste into a canonical kind and id:
// an hf:// URI, a hub URL in any of its forms, an arXiv link, an @handle, or a
// bare id.
//
// The two default rules are a deliberate bet: two path segments is a model, one
// is a namespace. That is right far more often than not, and every command that
// needs a different default names its kind explicitly, so the bet is never
// load-bearing.
func Classify(input string) (kind, id string, err error) {
	s := strings.TrimSpace(input)
	if s == "" {
		return "", "", errs.Usage("empty reference")
	}
	if k, i, ok := SplitURI(s); ok {
		return k, i, nil
	}
	if strings.HasPrefix(s, "@") {
		return KindNamespace, strings.TrimPrefix(s, "@"), nil
	}
	if strings.Contains(s, "://") {
		return classifyURL(s)
	}
	// A bare arXiv id: four digits, a dot, four or five digits.
	if looksLikeArxiv(s) {
		return KindPaper, s, nil
	}
	parts := strings.Split(strings.Trim(s, "/"), "/")
	switch len(parts) {
	case 1:
		if reservedPaths[parts[0]] {
			return "", "", errs.Usage("%q is a site path, not an entity", s)
		}
		return KindNamespace, parts[0], nil
	case 2:
		return KindModel, parts[0] + "/" + parts[1], nil
	default:
		return "", "", errs.Usage("unrecognized hf reference: %q", input)
	}
}

func classifyURL(raw string) (kind, id string, err error) {
	u, perr := url.Parse(raw)
	if perr != nil {
		return "", "", errs.Usage("bad URL %q: %v", raw, perr)
	}
	host := strings.ToLower(u.Hostname())
	if host == "arxiv.org" || host == "www.arxiv.org" {
		// /abs/2401.02412, /pdf/2401.02412v1
		seg := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(seg) >= 2 {
			return KindPaper, strings.TrimSuffix(trimVersion(seg[len(seg)-1]), ".pdf"), nil
		}
		return "", "", errs.Usage("no arXiv id in %q", raw)
	}
	if strings.HasSuffix(host, ".hf.space") {
		// A running space's own domain: owner-name.hf.space is not reversible
		// to owner/name, since both halves may contain dashes. Report it rather
		// than guessing wrong.
		return "", "", errs.Unsupported("a *.hf.space URL does not name its repo; use the huggingface.co page")
	}
	if !hostAliases[host] {
		return "", "", errs.Usage("%q is not a Hugging Face URL", raw)
	}
	seg := splitPath(u.Path)
	if len(seg) == 0 {
		return "", "", errs.Usage("no entity in %q", raw)
	}
	kind = KindModel
	if k, ok := pathKind[seg[0]]; ok {
		kind, seg = k, seg[1:]
	}
	switch kind {
	case KindPaper:
		if len(seg) == 0 {
			return "", "", errs.Usage("no paper id in %q", raw)
		}
		return KindPaper, trimVersion(seg[len(seg)-1]), nil
	case KindBlog, KindTask:
		if len(seg) == 0 {
			return "", "", errs.Usage("no slug in %q", raw)
		}
		return kind, strings.Join(seg, "/"), nil
	case KindPost:
		if len(seg) < 2 {
			return "", "", errs.Usage("no post id in %q", raw)
		}
		return KindPost, seg[0] + "/" + seg[1], nil
	case KindCollection:
		if len(seg) < 2 {
			return "", "", errs.Usage("no collection slug in %q", raw)
		}
		return KindCollection, seg[0] + "/" + seg[1], nil
	}

	// What is left is a repo path, possibly with a sub-page after it.
	if len(seg) == 1 {
		if reservedPaths[seg[0]] {
			return "", "", errs.Usage("%q is a site path, not an entity", raw)
		}
		return KindNamespace, seg[0], nil
	}
	repo := seg[0] + "/" + seg[1]
	rest := seg[2:]
	if len(rest) == 0 {
		return kind, repo, nil
	}
	switch rest[0] {
	case "discussions":
		if len(rest) >= 2 {
			return KindDiscussion, kind + "/" + repo + "#" + rest[1], nil
		}
		return kind, repo, nil
	case "blob", "raw", "resolve":
		if len(rest) >= 3 {
			return KindFile, kind + "/" + repo + "@" + rest[1] + "/" + strings.Join(rest[2:], "/"), nil
		}
	case "commit":
		if len(rest) >= 2 {
			return KindCommit, kind + "/" + repo + "@" + rest[1], nil
		}
	case "tree":
		if len(rest) >= 2 {
			return KindRef, kind + "/" + repo + "@" + rest[1], nil
		}
	}
	return kind, repo, nil
}

// Locate is the inverse of Classify for one resource: where it lives on the web.
func Locate(kind, id string) (string, error) {
	id = strings.Trim(id, "/")
	if id == "" {
		return "", errs.Usage("empty id")
	}
	switch kind {
	case KindModel:
		return BaseURL + "/" + id, nil
	case KindDataset:
		return BaseURL + "/datasets/" + id, nil
	case KindSpace:
		return BaseURL + "/spaces/" + id, nil
	case KindKernel:
		return BaseURL + "/kernels/" + id, nil
	case KindUser, KindOrg, KindNamespace:
		return BaseURL + "/" + id, nil
	case KindCollection:
		return BaseURL + "/collections/" + id, nil
	case KindPaper:
		return BaseURL + "/papers/" + id, nil
	case KindPost:
		return BaseURL + "/posts/" + id, nil
	case KindBlog:
		return BaseURL + "/blog/" + id, nil
	case KindTask:
		return BaseURL + "/tasks/" + id, nil
	case KindTag:
		return tagURL(id)
	case KindDiscussion:
		repoKind, repo, num, ok := splitDiscussionID(id)
		if !ok {
			return "", errs.Usage("bad discussion id %q", id)
		}
		base, err := Locate(repoKind, repo)
		if err != nil {
			return "", err
		}
		return base + "/discussions/" + num, nil
	case KindCommit:
		repoKind, repo, rev, _, ok := splitRevID(id)
		if !ok {
			return "", errs.Usage("bad commit id %q", id)
		}
		base, err := Locate(repoKind, repo)
		if err != nil {
			return "", err
		}
		return base + "/commit/" + rev, nil
	case KindRef:
		repoKind, repo, rev, _, ok := splitRevID(id)
		if !ok {
			return "", errs.Usage("bad ref id %q", id)
		}
		base, err := Locate(repoKind, repo)
		if err != nil {
			return "", err
		}
		return base + "/tree/" + strings.TrimPrefix(strings.TrimPrefix(rev, "refs/heads/"), "refs/tags/"), nil
	case KindFile:
		repoKind, repo, rev, path, ok := splitRevID(id)
		if !ok || path == "" {
			return "", errs.Usage("bad file id %q", id)
		}
		base, err := Locate(repoKind, repo)
		if err != nil {
			return "", err
		}
		return base + "/blob/" + rev + "/" + path, nil
	case KindSplit:
		ds, cfg, split, ok := splitSplitID(id)
		if !ok {
			return "", errs.Usage("bad split id %q", id)
		}
		return BaseURL + "/datasets/" + ds + "/viewer/" + url.PathEscape(cfg) + "/" + url.PathEscape(split), nil
	case KindProvider:
		return BaseURL + "/inference/models?inference_provider=" + url.QueryEscape(id), nil
	default:
		return "", errs.Usage("hf has no resource kind %q", kind)
	}
}

// tagURL points a tag at the listing it filters. A tag is only browsable
// through a repo search, so the location is that search.
func tagURL(id string) (string, error) {
	typ, val, ok := strings.Cut(id, ":")
	if !ok {
		return BaseURL + "/models?other=" + url.QueryEscape(id), nil
	}
	switch typ {
	case "license":
		return BaseURL + "/models?license=license:" + url.QueryEscape(val), nil
	case "language":
		return BaseURL + "/models?language=" + url.QueryEscape(val), nil
	case "library":
		return BaseURL + "/models?library=" + url.QueryEscape(val), nil
	case "pipeline_tag":
		return BaseURL + "/models?pipeline_tag=" + url.QueryEscape(val), nil
	case "dataset":
		return BaseURL + "/models?dataset=dataset:" + url.QueryEscape(val), nil
	default:
		return BaseURL + "/models?other=" + url.QueryEscape(val), nil
	}
}

// --- composite id encoding ---
//
// Three kinds have ids that embed their parent repo, because their own names
// are only unique within it: discussion numbers, commit shas, and file paths.

// DiscussionID builds model/owner/name#12.
func DiscussionID(repoKind, repo string, num int) string {
	return repoKind + "/" + repo + "#" + strconv.Itoa(num)
}

func splitDiscussionID(id string) (repoKind, repo, num string, ok bool) {
	head, num, ok := strings.Cut(id, "#")
	if !ok {
		return "", "", "", false
	}
	repoKind, repo, ok = cutRepoKind(head)
	return repoKind, repo, num, ok
}

// RevID builds model/owner/name@main/path/to/file, with the path optional.
func RevID(repoKind, repo, rev, path string) string {
	s := repoKind + "/" + repo + "@" + rev
	if path != "" {
		s += "/" + strings.TrimPrefix(path, "/")
	}
	return s
}

func splitRevID(id string) (repoKind, repo, rev, path string, ok bool) {
	head, tail, ok := strings.Cut(id, "@")
	if !ok {
		return "", "", "", "", false
	}
	repoKind, repo, ok = cutRepoKind(head)
	if !ok {
		return "", "", "", "", false
	}
	// A ref can itself contain slashes (refs/heads/main), so a revision that
	// starts with refs/ keeps its first three segments.
	if strings.HasPrefix(tail, "refs/") {
		seg := splitPath(tail)
		if len(seg) >= 3 {
			return repoKind, repo, strings.Join(seg[:3], "/"), strings.Join(seg[3:], "/"), true
		}
		return repoKind, repo, tail, "", true
	}
	rev, path, _ = strings.Cut(tail, "/")
	return repoKind, repo, rev, path, true
}

// SplitID builds dataset/config/split for a viewer split.
func SplitID(dataset, config, split string) string {
	return dataset + "/" + config + "/" + split
}

func splitSplitID(id string) (dataset, config, split string, ok bool) {
	seg := splitPath(id)
	if len(seg) != 4 {
		return "", "", "", false
	}
	return seg[0] + "/" + seg[1], seg[2], seg[3], true
}

// cutRepoKind peels the leading repo kind off a composite id.
func cutRepoKind(s string) (kind, repo string, ok bool) {
	kind, repo, ok = strings.Cut(s, "/")
	if !ok {
		return "", "", false
	}
	for _, k := range RepoKinds {
		if k == kind {
			return kind, repo, true
		}
	}
	return "", "", false
}

// --- small string helpers ---

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

// trimVersion drops the arXiv version suffix, so 2401.02412v3 and 2401.02412
// are one paper.
func trimVersion(s string) string {
	for i := len(s) - 1; i > 0; i-- {
		if s[i] < '0' || s[i] > '9' {
			if s[i] == 'v' && i < len(s)-1 {
				return s[:i]
			}
			return s
		}
	}
	return s
}

func looksLikeArxiv(s string) bool {
	dot := strings.IndexByte(s, '.')
	if dot != 4 || len(s) < 9 {
		return false
	}
	for i, r := range s {
		if i == 4 {
			continue
		}
		if r == 'v' && i > 8 {
			break
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
