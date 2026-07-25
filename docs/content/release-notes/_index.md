---
title: "Release notes"
linkTitle: "Release notes"
description: "What changed in each hf release, newest first."
weight: 40
---

What shipped in each release, newest first. Every tagged version builds the same
set of artifacts: archives for Linux, macOS, Windows, and FreeBSD, Linux
packages (deb, rpm, apk), a multi-arch container image on GHCR, and entries for
the package managers. Binaries are pure Go, so there is nothing to install
alongside them.

## v0.1.0

First release. hf reads huggingface.co and gives back records rather than pages.

**Records.** Every entity the hub publishes has a typed record carrying every
field its source returned: model, dataset, space, kernel, user, org, collection,
paper, post, blog entry, discussion, commit, ref, file, tag, task, split, and
inference provider. Each record has a canonical `hf://` address and lists the
URLs that contributed to it, so a surprising value can be traced back to
whatever said it. Anything upstream sends that this version does not model lands
in `extra` as raw JSON rather than being dropped.

**Commands.** Around sixty of them over one output contract. Read one thing,
list many, look inside a repository, look inside a dataset, walk the graph,
export the lot. `hf get` takes a bare id, a URL from a browser, or an `hf://`
URI, works out what it points at, and reads it.

**The dataset viewer** is a first-class surface: `splits`, `schema`, `head`,
`rows`, `stats`, `size`, `parquet`, `filter`, `dsearch`, `valid`, and
`croissant`. You can look at the actual data without downloading a shard.

**The page plane** covers what the API does not return. `hf page` parses a
rendered page into JSON one for one with what a browser gets, organised by
component rather than by DOM node, and `--deep` merges those fields into a
normal read: decoded tag objects with human labels, a space's runtime state, an
org's whole member list, a paper's AI summary.

**The graph plane** emits nodes and edges with a source on every edge, so a
consumer can decide how much to trust each one. `hf crawl` walks breadth-first
from a seed under a budget, and `hf rdf` and `hf export` write the result as
N-Triples, Turtle, JSON-LD, or N-Quads over schema.org terms where they exist.

**Serving.** `hf serve` puts the same operations behind HTTP as NDJSON, `hf mcp`
exposes them to agents, and any Go program that imports the package can
dereference `hf://` URIs. All three come from the same registrations as the CLI.

Full detail in the
[changelog](https://github.com/tamnd/hf-cli/blob/main/CHANGELOG.md).
