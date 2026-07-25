# hf

Read huggingface.co as data.

`hf` is a single pure-Go binary that turns the hub into records. Every model,
dataset, space, kernel, user, org, collection, paper, post, blog entry, and
discussion gets a canonical `hf://` address, a typed record carrying every field
its source returned, and a set of edges to the other things it names. Read one
entity, list a million, walk the graph between them, or export the lot as RDF.

No API key for public data, nothing to run alongside it.

## Install

```bash
go install github.com/tamnd/hf-cli/cmd/hf@latest   # Go
brew install --cask tamnd/tap/hf                   # macOS
scoop bucket add tamnd https://github.com/tamnd/scoop-bucket && scoop install hf
docker run --rm ghcr.io/tamnd/hf:latest --help     # container
```

Or grab a prebuilt binary from the
[releases](https://github.com/tamnd/hf-cli/releases). Linux has a signed apt and
dnf repository, covered in the
[install docs](https://tamnd.github.io/hf-cli/getting-started/installation/).

## Read one thing

```bash
hf model google-bert/bert-base-uncased        # every field the API returns
hf model google-bert/bert-base-uncased --deep # plus the fields only the page has
hf dataset rajpurkar/squad --card             # plus the README body
hf space black-forest-labs/FLUX.1-dev         # runtime state included
hf paper 1810.04805                           # authors and what cites it
hf get https://huggingface.co/google/gemma-3-27b-it   # paste anything
```

`hf get` takes a bare id, a URL you copied out of a browser, or an `hf://` URI,
works out what it points at, and reads it. Every other read command is the same
thing with the kind already decided.

## List many

```bash
hf models --author google --task text-generation --sort downloads -n 100
hf datasets --search squad
hf search bert                    # every entity kind at once
hf trending --type model
hf papers --daily
hf collections --item models/google-bert/bert-base-uncased
```

Listing streams: `-n` stops early without fetching the rest, and no command
holds a full result set in memory unless a format forces it to.

## Walk the graph

```bash
hf graph google-bert/bert-base-uncased        # the node and its edges
hf edges google-bert/bert-base-uncased        # just the edges
hf crawl google/gemma-3-27b-it --depth 2      # breadth-first from a seed
hf children google-bert/bert-base-uncased     # what was fine-tuned from it
hf parents  Jorgeutd/bert-base-uncased-finetuned-surveyclassification
hf citations 1810.04805                       # every repo tagged with a paper
```

Edges come from four sources: explicit id references in the API payloads,
namespace prefixes on repo ids, decoded tags, and README front matter. Each edge
records which one it came from, so a consumer can decide how much to trust it.

## Export it

```bash
hf rdf google-bert/bert-base-uncased --format ttl
hf rdf google-bert/bert-base-uncased --format jsonld
hf export google/gemma-3-27b-it --depth 2 --format jsonl > gemma.jsonl
hf croissant rajpurkar/squad          # the hub's own Croissant document
```

RDF comes out as N-Triples, Turtle, JSON-LD, or N-Quads, over `schema.org` where
a term exists and an `hf:` namespace where none does. N-Triples and N-Quads
stream, so a crawl of a large org never needs the graph in memory.

## Look inside a repository

```bash
hf tree google-bert/bert-base-uncased --recursive
hf files rajpurkar/squad
hf cat google-bert/bert-base-uncased config.json
hf readme rajpurkar/squad
hf card rajpurkar/squad               # the parsed front matter
hf commits google-bert/bert-base-uncased -n 20
hf refs google-bert/bert-base-uncased
```

## Look inside a dataset

The dataset viewer is a second API with its own host, and `hf` treats it as part
of the same tool:

```bash
hf splits rajpurkar/squad
hf schema rajpurkar/squad --split train
hf rows   rajpurkar/squad --split train --offset 0 -n 5
hf stats  rajpurkar/squad --split train
hf dsearch rajpurkar/squad "beyonce"
hf filter rajpurkar/squad "title = 'Beyonce'"
hf parquet rajpurkar/squad
```

## The page plane

Parts of the hub have no API at all, and other parts return more to a browser
than to `/api`. `hf` reads the rendered page for those, pulling the JSON the
page already carries rather than scraping text:

```bash
hf page google-bert/bert-base-uncased   # 1:1 with what the browser gets
hf model google-bert/bert-base-uncased --deep
```

`--deep` is what gets you decoded tag objects with human labels, discussion
counts, linked spaces with their live running state, paper AI summaries, and an
org's whole profile in one request instead of seven.

## Output

Every command shares one contract: `-o table|json|jsonl|csv|tsv|url|raw`,
`--fields` to pick columns, `--template` for a custom line, `-n` to limit. The
default adapts to where output goes, a table on a terminal and JSONL in a pipe,
so the same command reads well by hand and parses cleanly downstream.

`-o url` is the one the others are measured against. It prints one URL per
record and nothing else, so this composes with no glue:

```bash
hf models --author google -n 50 -o url | xargs -n1 hf get
```

## Auth

Public reads need no token. For private repos and `hf whoami`, pass `--token` or
set `$HF_TOKEN`, `$HUGGING_FACE_HUB_TOKEN`, or a token file at `$HF_HOME/token`.

## Serve it

The same operations are available over HTTP and as an MCP tool set for agents,
with no extra code:

```bash
hf serve --addr :7777    # GET /v1/model/<ref> returns NDJSON
hf mcp                   # speak MCP over stdio
```

## Use it as a resource-URI driver

`hf` registers a `hf` domain the way a program registers a database driver with
`database/sql`. A host enables it with one blank import:

```go
import _ "github.com/tamnd/hf-cli/hf"
```

Then [ant](https://github.com/tamnd/ant) (or any program that links the package)
dereferences `hf://` URIs without knowing anything about the hub:

```bash
ant get hf://model/google-bert/bert-base-uncased
ant cat hf://model/google-bert/bert-base-uncased
ant ls  hf://org/google
ant url hf://dataset/rajpurkar/squad
```

## Development

```
cmd/hf/   thin main: hands cli.NewApp to kit.Run
cli/      assembles the kit App and registers every operation
hf/       the library: client, records, page plane, graph, RDF, domain.go
hftest/   records real exchanges with the hub and replays them offline
docs/     tago documentation site
```

```bash
make build      # ./bin/hf
make test       # go test ./..., offline, replays the recorded fixtures
make vet        # go vet ./...
make fixtures   # re-record against the live hub, then refresh the goldens
```

The tests run against the hub's real answers. `hf/testdata/live` holds a
recorded exchange for every command in the table, and the offline run replays
them, checks that no record dropped a field into `Extra`, and compares the set
of populated field paths against a golden. Values are deliberately not pinned: a
download count changes hourly, and a golden full of them stops being a test.

Re-record with `make fixtures` when the hub changes something, then read the
golden diff before committing it. Recording is anonymous, so the fixtures are
what any reader sees.

## Releasing

Push a version tag and GitHub Actions runs GoReleaser, which builds the
archives, Linux packages, the multi-arch GHCR image, checksums, SBOMs, and a
cosign signature:

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push --tags
```

The Homebrew and Scoop steps self-disable until their tokens exist, so a release
works with no extra secrets. Record what changed in
[CHANGELOG.md](CHANGELOG.md) and on the release notes page in `docs/` before
tagging.

## License

Apache-2.0. See [LICENSE](LICENSE).
