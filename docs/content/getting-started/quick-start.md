---
title: "Quick start"
description: "Read your first record, then walk the graph."
weight: 30
---

Once `hf` is on your `PATH`, read a model:

```bash
hf model google-bert/bert-base-uncased
```

The argument is a ref, and every command that takes one accepts every form of
it. A bare name, an owner-qualified id, a URL you copied out of a browser, and a
canonical URI are all the same command:

```bash
hf model bert-base-uncased
hf model google-bert/bert-base-uncased
hf model https://huggingface.co/google-bert/bert-base-uncased
hf model hf://model/google-bert/bert-base-uncased
```

If you do not know what kind of thing a ref is, `hf get` works it out:

```bash
hf get rajpurkar/squad
hf get https://huggingface.co/collections/fdtn-ai/antares-6a5804c889e78b51c447c38a
```

## Shape the output

The same flags work on every command:

```bash
hf model google-bert/bert-base-uncased --fields id,downloads,likes,pipeline_tag
hf model google-bert/bert-base-uncased -o json | jq .safetensors
hf models --author google -o url | head -5
```

`-o` takes `table`, `json`, `jsonl`, `csv`, `tsv`, `url`, or `raw`. Left to
`auto`, it prints a table to a terminal and JSONL into a pipe, so the same
command reads well by hand and parses cleanly downstream. See
[output formats](/reference/output/) for the full contract.

## List and search

```bash
hf models --author google --task text-generation --sort downloads -n 20
hf datasets --search squad -n 10
hf search bert                     # every entity kind at once
hf trending --type model
```

Listing streams. `-n` stops early without fetching the next page, and leaving it
off means the whole result set, which for `hf models` is a few million records
and a very long afternoon.

## Go deeper

The API does not return everything the page shows. `--deep` fetches the rendered
page as well and merges the fields only it carries:

```bash
hf model google-bert/bert-base-uncased --deep -o json | jq .tagObjs
hf org google --deep -o json | jq '.models | length'
```

`--card` adds the README body, and `hf card` gives you just the parsed front
matter.

## Walk the graph

Every record knows what it points at:

```bash
hf graph google-bert/bert-base-uncased        # the node and its edges
hf edges google-bert/bert-base-uncased        # edges only
hf children google-bert/bert-base-uncased     # what was fine-tuned from it
hf crawl hf://org/google --depth 2 -n 500
```

See [walk the graph](/guides/graph/) for what the edges mean and where each one
comes from.

## Export it

```bash
hf rdf google-bert/bert-base-uncased --format ttl
hf export hf://org/google --depth 2 --format jsonl > google.jsonl
hf croissant rajpurkar/squad
```

See [linked data](/guides/linked-data/) for the vocabulary.

## Serve it instead

The same operations are available over HTTP and to agents over MCP:

```bash
hf serve --addr :7777 &
curl -s 'localhost:7777/v1/model/google-bert/bert-base-uncased'   # NDJSON
hf mcp                                                            # MCP over stdio
```

## Where to go next

- [Walk the graph](/guides/graph/)
- [Export linked data](/guides/linked-data/)
- [Read a dataset](/guides/datasets/)
- [The CLI reference](/reference/cli/)
