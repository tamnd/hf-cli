package hf

import (
	"net/url"
	"strings"
	"time"
)

// tree.go covers what is inside a repo: refs, files, commits, and the scan
// verdicts the hub attaches to files.

// Ref is one branch, tag, or conversion ref.
type Ref struct {
	Meta

	RepoID       string `json:"repoId,omitempty" table:"-"`
	RepoType     string `json:"repoType,omitempty" table:"-"`
	Name         string `json:"name" table:"name"`
	Ref          string `json:"ref" table:"-"`
	Kind         string `json:"kind" table:"kind"`
	TargetCommit string `json:"targetCommit,omitempty" table:"commit"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (r *Ref) UnmarshalJSON(b []byte) error {
	type raw Ref
	return decodeExtra(b, (*raw)(r), &r.Extra)
}

func (r *Ref) normalize(repoKind, repo, kind, sourceURL string) {
	r.RepoType, r.RepoID = repoKind, repo
	if r.Kind == "" {
		r.Kind = kind
	}
	if r.Ref == "" && r.Name != "" {
		switch r.Kind {
		case "branch":
			r.Ref = "refs/heads/" + r.Name
		case "tag":
			r.Ref = "refs/tags/" + r.Name
		case "convert":
			r.Ref = "refs/convert/" + r.Name
		}
	}
	r.setMeta(KindRef, RevID(repoKind, repo, r.Name, ""), sourceURL)
	if u, err := repoURL(repoKind, repo); err == nil && r.Name != "" {
		r.URL = u + "/tree/" + url.PathEscape(r.Name)
	}
}

// TreeEntry is one node of a repo tree, from /tree or /paths-info.
type TreeEntry struct {
	Meta

	RepoID   string `json:"repoId,omitempty" table:"-"`
	RepoType string `json:"repoType,omitempty" table:"-"`
	Revision string `json:"revision,omitempty" table:"-"`

	Type string `json:"type" table:"type"`
	OID  string `json:"oid,omitempty" table:"oid"`
	Size int64  `json:"size,omitempty" table:"size"`
	Path string `json:"path" table:"path"`

	LFS        *LFSInfo        `json:"lfs,omitempty" table:"-"`
	LastCommit *CommitRef      `json:"lastCommit,omitempty" table:"-"`
	Security   *SecurityStatus `json:"securityFileStatus,omitempty" table:"-"`
	XetHash    string          `json:"xetHash,omitempty" table:"-"`

	// Derived, because a tree entry with no way to fetch its bytes is half a
	// record.
	DownloadURL string `json:"downloadUrl,omitempty" table:"-"`
	RawURL      string `json:"rawUrl,omitempty" table:"-"`
	IsLFS       bool   `json:"isLfs,omitempty" table:"-"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (t *TreeEntry) UnmarshalJSON(b []byte) error {
	type raw TreeEntry
	return decodeExtra(b, (*raw)(t), &t.Extra)
}

func (t *TreeEntry) normalize(repoKind, repo, rev, sourceURL string) {
	t.RepoType, t.RepoID, t.Revision = repoKind, repo, rev
	t.IsLFS = t.LFS != nil
	if t.LFS != nil && t.Size == 0 {
		t.Size = t.LFS.Size
	}
	base, err := repoURL(repoKind, repo)
	if err == nil && t.Type == "file" {
		t.DownloadURL = base + "/resolve/" + escapeRev(rev) + "/" + escapePath(t.Path)
		t.RawURL = base + "/raw/" + escapeRev(rev) + "/" + escapePath(t.Path)
	}
	t.setMeta(KindFile, RevID(repoKind, repo, rev, t.Path), sourceURL)
	if err == nil {
		kind := "blob"
		if t.Type == "directory" {
			kind = "tree"
		}
		t.URL = base + "/" + kind + "/" + escapeRev(rev) + "/" + escapePath(t.Path)
	}
}

// Dir reports whether the entry is a directory, which is what a recursive walk
// needs to decide whether to descend.
func (t *TreeEntry) Dir() bool { return t.Type == "directory" }

// LFSInfo is the pointer record for a large file. Size here is the real size,
// while an LFS file's git blob is only a few hundred bytes.
type LFSInfo struct {
	OID         string `json:"oid"`
	Size        int64  `json:"size"`
	PointerSize int    `json:"pointerSize,omitempty"`
}

// CommitRef is the last commit that touched a path.
type CommitRef struct {
	ID    string    `json:"id"`
	Title string    `json:"title,omitempty"`
	Date  time.Time `json:"date,omitzero"`
}

// SecurityStatus is the scan verdict set. Each scanner is optional and a repo
// can have any subset, so absence means not scanned rather than clean.
type SecurityStatus struct {
	Status           string      `json:"status,omitempty"`
	ProtectAIScan    *ScanResult `json:"protectAiScan,omitempty"`
	AVScan           *ScanResult `json:"avScan,omitempty"`
	PickleImportScan *PickleScan `json:"pickleImportScan,omitempty"`
	JFrogScan        *ScanResult `json:"jfrogScan,omitempty"`
}

// Unsafe reports whether any scanner flagged the file.
func (s *SecurityStatus) Unsafe() bool {
	if s == nil {
		return false
	}
	if s.Status == "unsafe" || s.Status == "suspicious" {
		return true
	}
	for _, r := range []*ScanResult{s.ProtectAIScan, s.AVScan, s.JFrogScan} {
		if r != nil && (r.Status == "unsafe" || r.Status == "suspicious") {
			return true
		}
	}
	if s.PickleImportScan != nil {
		for _, imp := range s.PickleImportScan.Imports {
			if imp.Safety == "dangerous" || imp.Safety == "suspicious" {
				return true
			}
		}
	}
	return false
}

// ScanResult is one scanner's verdict.
type ScanResult struct {
	Status     string `json:"status"`
	Message    string `json:"message,omitempty"`
	ReportLink string `json:"reportLink,omitempty"`
}

// PickleScan lists what a pickle file would import if it were loaded, which is
// the only useful thing to know about a pickle before loading it.
type PickleScan struct {
	Status  string         `json:"status"`
	Imports []PickleImport `json:"pickleImports,omitempty"`
}

// PickleImport is one module and symbol a pickle references.
type PickleImport struct {
	Module string `json:"module"`
	Name   string `json:"name"`
	Safety string `json:"safety,omitempty"`
}

// Commit is one commit on a repo.
type Commit struct {
	Meta

	RepoID    string         `json:"repoId,omitempty" table:"-"`
	RepoType  string         `json:"repoType,omitempty" table:"-"`
	ID        string         `json:"id" table:"id"`
	Title     string         `json:"title" table:"title,truncate"`
	Message   string         `json:"message,omitempty" table:"-"`
	Date      time.Time      `json:"date,omitzero" table:"date,time"`
	Authors   []CommitAuthor `json:"authors,omitempty" table:"-"`
	Formatted string         `json:"formatted,omitempty" table:"-"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (c *Commit) UnmarshalJSON(b []byte) error {
	type raw Commit
	return decodeExtra(b, (*raw)(c), &c.Extra)
}

func (c *Commit) normalize(repoKind, repo, sourceURL string) {
	c.RepoType, c.RepoID = repoKind, repo
	for i := range c.Authors {
		c.Authors[i].normalize()
	}
	c.setMeta(KindCommit, RevID(repoKind, repo, c.ID, ""), sourceURL)
	if u, err := repoURL(repoKind, repo); err == nil {
		c.URL = u + "/commit/" + c.ID
	}
}

// CommitAuthor is who wrote a commit. It is a hub account only when the commit
// email matched one, which is why the name can be empty.
type CommitAuthor struct {
	User   string `json:"user,omitempty"`
	Avatar string `json:"avatar,omitempty"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
	URI    string `json:"uri,omitempty"`
}

func (a *CommitAuthor) normalize() {
	if a.User != "" {
		a.URI = URI(KindNamespace, a.User)
	}
	if strings.HasPrefix(a.Avatar, "/") {
		a.Avatar = BaseURL + a.Avatar
	}
}

// escapeRev escapes a revision for a URL path. A ref such as refs/pr/1 keeps
// its slashes, because that is how the hub addresses it.
func escapeRev(rev string) string {
	if rev == "" {
		return "main"
	}
	parts := strings.Split(rev, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return strings.Join(parts, "/")
}

// escapePath escapes a repo-relative file path segment by segment.
func escapePath(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		parts[i] = url.PathEscape(s)
	}
	return strings.Join(parts, "/")
}

// repoURL is Locate for the four repo kinds, without the error handling noise
// at every call site inside this file.
func repoURL(kind, repo string) (string, error) {
	if kind == "" {
		kind = KindModel
	}
	return Locate(kind, repo)
}
