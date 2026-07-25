# Changelog

All notable changes to hf are recorded here.
The format follows [Keep a Changelog](https://keepachangelog.com/), and the project aims to follow [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.2.0] - 2026-07-25

First release of the rewrite.
hf reads huggingface.co and gives back records rather than pages.

The version starts at 0.2.0 because v0.1.0 was tagged on the earlier daily-papers tool and is already published in the Go checksum database, where a tag cannot be moved.

### Added

- Typed records for every entity the hub publishes: model, dataset, space, kernel, user, org, collection, paper, post, blog entry, discussion, commit, ref, file, tag, task, split, and inference provider.
  Each one carries every field its source returned, a canonical `hf://` address, and the list of URLs that contributed to it.
- `Extra`, a raw JSON map on every record holding anything upstream sent that this version does not model, so a hub change shows up rather than disappearing.
- Around sixty commands over one output contract: read one thing, list many, look inside a repository, look inside a dataset, walk the graph, export it.
- The dataset viewer as a first-class surface: `splits`, `schema`, `head`, `rows`, `stats`, `size`, `parquet`, `filter`, `dsearch`, `valid`, and `croissant`, so you can read the data without downloading a shard.
- A page plane for what the API does not return.
  `hf page` parses a rendered page into JSON one for one with what a browser gets, organised by component, and `--deep` merges those fields into a normal read.
- A graph plane.
  `hf graph`, `hf edges`, `hf crawl`, `hf children`, `hf parents`, and `hf citations` emit nodes and edges, each edge tagged with where it came from: an API id, a tag, a card, a page, or a derivation.
- RDF output as N-Triples, Turtle, JSON-LD, and N-Quads, over schema.org where a term exists and an `hf:` namespace where none does.
  Croissant documents are passed through rather than reimplemented.
- `hf serve` and `hf mcp`, which expose the same operations over HTTP and to agents with no extra code, and `hf://` URI dereferencing for any host that imports the package.
- A client that paces itself, retries on 429 and 5xx honouring `Retry-After`, caches on disk, paginates over the `Link` header, and maps every failure to a distinct exit code.
- Release artifacts: archives for Linux, macOS, Windows, and FreeBSD, deb, rpm and apk packages, a multi-arch GHCR image, SBOMs, and a cosign-signed checksum file.

[Unreleased]: https://github.com/tamnd/hf-cli/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/tamnd/hf-cli/releases/tag/v0.2.0
