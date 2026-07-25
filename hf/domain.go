package hf

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/tamnd/any-cli/kit"
)

// domain.go is the seam between this library and the kit framework. It declares
// what the hub is called, how its addresses are parsed, and how a client is
// built from the resolved config. Everything a person can type is registered in
// ops.go; nothing else in the package imports kit.

// Domain is the kit driver for huggingface.co. A blank import of this package
// enables it in any multi-domain host, the way a database driver registers
// itself, and the same Domain builds the single hf binary.
type Domain struct{}

func init() { kit.Register(Domain{}) }

// Info names the domain and every hostname that means it. The aliases matter:
// hf.co is a real redirect the hub itself hands out, so a pasted hf.co link has
// to resolve here rather than fall through as an unknown site.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme:  Scheme,
		Aliases: []string{"huggingface"},
		Hosts:   []string{Host, "www." + Host, "hf.co", "www.hf.co"},
		Identity: kit.Identity{
			Binary: "hf",
			Short:  "Discover everything on Hugging Face as structured data",
			Long: "hf reads huggingface.co and gives back records rather than pages.\n\n" +
				"Every model, dataset, space, kernel, user, org, collection, paper, post,\n" +
				"blog entry, and discussion the hub publishes has a canonical hf:// address,\n" +
				"a typed record with every field its source returned, and a set of edges to\n" +
				"the other things it names. Read one entity, list a million, walk the graph\n" +
				"between them, or export the whole thing as RDF.",
			Site: BaseURL,
			Repo: "https://github.com/tamnd/hf-cli",
		},
	}
}

// Classify satisfies kit.Resolver. It is the package's own parser, so a URI
// typed at a multi-domain host and one typed at hf are read by the same code.
func (Domain) Classify(input string) (uriType, id string, err error) {
	return Classify(input)
}

// Locate satisfies kit.Resolver: the https location of one resource.
func (Domain) Locate(uriType, id string) (string, error) {
	return Locate(uriType, id)
}

// Defaults overlays this domain's baseline onto the framework's. The hub is
// generous and the numbers here are the polite ones from the spec: a request
// every 150ms, five retries, eight workers.
func Defaults(c *kit.Config) {
	c.Rate = 150 * time.Millisecond
	c.Retries = 5
	c.Workers = 8
	c.Timeout = 30 * time.Second
	c.UserAgent = DefaultUserAgent
}

// flags holds the domain's own global flags. kit resolves the framework globals
// (--limit, --rate, --timeout, --db, --no-cache) itself; these are the ones only
// this tool has, and they are read once when the client is built.
//
// Package-level state is the framework's contract here: GlobalFlags binds to the
// domain's variables and the client factory reads them, and there is exactly one
// run per process.
var flags struct {
	token string
	deep  bool
	card  bool
	jobs  int
	cache string
}

// Register installs the client factory, the domain globals, and every operation.
// It does no I/O and is deterministic, so a host can call it at startup.
func (d Domain) Register(app *kit.App) {
	app.SetClient(newClientFor)
	app.GlobalFlags(bindFlags)
	registerOps(app)
}

func bindFlags(f *kit.FlagSet) {
	f.StringVar(&flags.token, "token", "", "hub token; also read from $HF_TOKEN, $HUGGING_FACE_HUB_TOKEN, $HF_HOME/token")
	f.BoolVar(&flags.deep, "deep", false, "also fetch the rendered page and merge the fields only it carries")
	f.BoolVar(&flags.card, "card", false, "include the README body on repo records")
	f.IntVarP(&flags.jobs, "jobs", "j", 0, "concurrent requests (0 = the default 8)")
	f.StringVar(&flags.cache, "cache", "", "response cache directory (default under the data dir)")
}

// newClientFor builds the one client a run shares. Every command reaches it
// through a kit:"inject" field, so pacing and the cache are shared across a
// whole pipeline rather than per command.
func newClientFor(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	c.HTTP = &http.Client{Timeout: cfg.Timeout}
	c.Rate = cfg.Rate
	c.Retries = cfg.Retries
	c.Workers = cfg.Workers
	c.NoCache = cfg.NoCache
	c.CacheDir = filepath.Join(cfg.CacheDir, "http")
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if flags.token != "" {
		c.Token = flags.token
	}
	if flags.cache != "" {
		c.CacheDir = flags.cache
	}
	if flags.jobs > 0 {
		c.Workers = flags.jobs
	}
	c.Deep = flags.deep
	c.Card = flags.card
	return c, nil
}
