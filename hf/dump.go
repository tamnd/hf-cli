package hf

import (
	"context"
	"sync"

	"github.com/tamnd/any-cli/kit/errs"
)

// dump.go is the crawl-the-world command. It exists mostly so the politeness
// budget has one obvious place to live: everything here is about not sending
// two million requests to someone else's servers by accident.

// DumpOptions bounds a full sweep of one kind.
type DumpOptions struct {
	// Kind is one of the entity kinds. It selects both the sitemap to walk and
	// the record to fetch per id.
	Kind string
	// Limit stops after n records. Zero means the whole kind, which for models
	// is around 2.4 million and a very long afternoon.
	Limit int
	// Workers is the fetch fan-out. The client's pacer still serialises the
	// actual requests, so this trades latency, not politeness.
	Workers int
	// Yes confirms a sweep whose estimate exceeds ConfirmAbove.
	Yes bool
	// IDsOnly skips the detail fetch and emits the sitemap entry, which is the
	// cheap way to answer "what exists" without asking about each one.
	IDsOnly bool
}

// ConfirmAbove is the request count past which a dump refuses to start without
// an explicit yes. A tool that can accidentally send two million requests should
// say so first.
const ConfirmAbove = 100000

// dumpSitemap maps an entity kind to the sitemap that enumerates it. Only these
// four are published; everything else has to be reached through a list endpoint.
var dumpSitemap = map[string]string{
	KindModel:   "models",
	KindDataset: "datasets",
	KindSpace:   "spaces",
	KindUser:    "users",
}

// Dump walks every id of one kind and emits its record. Ids come from the
// sitemap rather than a list cursor, because a cursor has to stay valid for the
// length of the run and a sitemap does not.
func (c *Client) Dump(ctx context.Context, o DumpOptions, emit func(any) error) error {
	which, ok := dumpSitemap[o.Kind]
	if !ok {
		return errs.Unsupported("the hub publishes no sitemap for %s; use hf %ss instead", o.Kind, o.Kind)
	}
	if !o.Yes && o.Limit == 0 && !o.IDsOnly {
		return errs.Usage("a full %s dump is well over %d requests; pass --yes to start it, or -n to bound it", o.Kind, ConfirmAbove)
	}
	workers := o.Workers
	if workers <= 0 {
		workers = c.Workers
	}
	if workers <= 0 {
		workers = 1
	}

	// One mutex around emit rather than a fan-in channel: the sink is the only
	// shared thing and the fetches are what take the time.
	var mu sync.Mutex
	send := func(rec any) error {
		mu.Lock()
		defer mu.Unlock()
		return emit(rec)
	}

	ids := make(chan SitemapEntry)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	fail := func(err error) {
		errMu.Lock()
		defer errMu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ent := range ids {
				kind, id, err := Classify(ent.Loc)
				if err != nil {
					continue
				}
				rec, err := c.Fetch(ctx, kind, id)
				if err != nil {
					// A gone or gated repo mid-sweep is normal and is not worth
					// ending a run that has already produced millions of rows.
					if IsNotFound(err) || IsNeedAuth(err) {
						continue
					}
					fail(err)
					cancel()
					return
				}
				if err := send(rec); err != nil {
					fail(err)
					cancel()
					return
				}
			}
		}()
	}

	walkErr := c.Sitemap(ctx, which, o.Limit, func(e *SitemapEntry) error {
		if o.IDsOnly {
			return send(e)
		}
		select {
		case ids <- *e:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	close(ids)
	wg.Wait()

	errMu.Lock()
	defer errMu.Unlock()
	if firstErr != nil {
		return firstErr
	}
	if walkErr != nil && ctx.Err() == nil {
		return walkErr
	}
	return nil
}
