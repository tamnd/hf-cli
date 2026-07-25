---
title: "Introduction"
description: "What hf is and how it is put together."
weight: 10
---

`hf` turns huggingface.co into structured data.

The hub is a large, well-populated knowledge graph that happens to be served as
a website with an API bolted to the side. Models cite papers, datasets train
models, spaces load both, orgs own all of it, and every one of those relations
is already written down somewhere in the payloads. `hf` reads them, gives each
entity a canonical address, and hands you the graph.

## What it covers

Repositories of four kinds: model, dataset, space, and kernel. Namespaces of
two: user and org. Then collections, papers, posts, blog entries, discussions,
commits, refs, files, tags, tasks, and dataset splits. Every one is a typed
record with a `hf://` URI and a set of edges.

## Two data planes

- **The API plane.** `huggingface.co/api` for entities, and
  `datasets-server.huggingface.co` for the dataset viewer, which is a second API
  with its own host and its own idea of pagination.
- **The page plane.** The rendered pages carry their data as JSON in
  `data-props` attributes, which is how the site hydrates itself. Some of that
  data has no API route at all, and some of it is richer than the API's version.

`hf` prefers the API when the API has the field and reaches for the page when it
does not. `--deep` merges both into one record.

## Nothing is dropped

Every record type decodes the fields it models and sweeps the rest into an
`extra` map rather than discarding them. The test suite replays recorded
responses from the live hub and fails when anything lands in `extra`, so a field
the hub starts returning shows up as a failing test rather than as data you
never saw.

## How it is built

- A **library package** (`hf`) holds the HTTP client, the typed records, the
  page parser, the graph extractor, and the RDF writers. It paces requests, sets
  an honest User-Agent, and retries what a public site throws under load.
- **`hf/ops.go`** declares each operation once on the
  [any-cli/kit](https://github.com/tamnd/any-cli) framework. That single
  declaration becomes a CLI command, an HTTP route, an MCP tool, and a
  resource-URI dereference.
- A thin **`cmd/hf`** hands the assembled app to `kit.Run`.

## One operation, four surfaces

```bash
hf model google-bert/bert-base-uncased             # the command
hf serve --addr :7777                              # GET /v1/model/<ref>
hf mcp                                             # the model tool, over stdio
ant get hf://model/google-bert/bert-base-uncased   # the URI dereference
```

## Scope

`hf` is a read-only client. It does not upload, create, or delete anything. That
narrow scope keeps it a single small binary with no database, no daemon, and no
setup.

Next: [install it](/getting-started/installation/), then take the
[quick start](/getting-started/quick-start/).
