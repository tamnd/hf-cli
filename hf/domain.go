package hf

import (
	"context"

	"github.com/tamnd/any-cli/kit"
)

// domain.go exposes hf as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/hf-cli/hf"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// hf:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone hf binary, so the binary and a host share
// one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the hf driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against,
// and the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "hf",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "hf",
			Short:  "A command line for Hugging Face.",
			Long:   `A command line for Hugging Face. Browse daily papers and trending models. No API key required.`,
			Site:   "https://" + Host,
			Repo:   "https://github.com/tamnd/hf-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "top", Group: "papers", List: true,
		URIType: "paper", Summary: "List top daily papers from Hugging Face"}, topCmd)
}

// newClient builds the client from the host-resolved config.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

type topIn struct {
	Date   string  `kit:"flag" help:"date in YYYY-MM-DD format (default: yesterday UTC)"`
	Limit  int     `kit:"flag,inherit" help:"max results"`
	Client *Client `kit:"inject"`
}

func topCmd(ctx context.Context, in topIn, emit func(*Paper) error) error {
	papers, err := in.Client.Top(ctx, in.Date, in.Limit)
	if err != nil {
		return err
	}
	for _, p := range papers {
		if err := emit(p); err != nil {
			return err
		}
	}
	return nil
}
