---
title: "CLI"
description: "Every command, grouped the way the help output groups them."
weight: 10
---

```
hf <command> [arguments] [flags]
```

Run `hf <command> --help` for the full flag list on any command.
Every command that takes a `<ref>` accepts a bare name, an owner-qualified id, a huggingface.co URL, or an `hf://` URI.

## Read one thing

| Command | What it does |
|---|---|
| `get <ref>` | Read whatever a reference points at |
| `model <ref>` | One model, every field the hub returns |
| `dataset <ref>` | One dataset |
| `space <ref>` | One space, runtime state included |
| `kernel <ref>` | One compute kernel |
| `user <name>` | One user profile |
| `org <name>` | One organization |
| `ns <name>` | A name without knowing whether it is a user or an org |
| `collection <slug>` | One collection and its items |
| `paper <id>` | One paper, its authors, and what cites it |
| `post <ref>` | One community post |
| `blog <slug>` | One blog entry, body included |
| `discussion <repo> <num>` | One discussion or pull request, with its event thread |
| `whoami` | Who the current token belongs to |

## List

| Command | Key flags |
|---|---|
| `models` | `--author`, `--search`, `--filter`, `--task`, `--library`, `--language`, `--license`, `--sort`, `--direction`, `--full` |
| `datasets` | the same, plus `--benchmark`, `--size` |
| `spaces` | the same, plus `--sdk` |
| `kernels` | `--author`, `--search`, `--sort` |
| `collections` | `--owner`, `--item`, `--sort` |
| `papers` | `--search`, `--date`, `--daily`, `--sort` |
| `posts` | community posts |
| `blogs` | from the RSS index |
| `discussions <repo>` | `--status`, `--type`, `--author` |
| `search <query>` | `--type`, across every entity kind |
| `trending` | `--type` |

The filter flags are sugar over the one `filter=` parameter the API takes, joined with AND, so `--task text-generation --library transformers` and `--filter text-generation --filter transformers` are the same request.

`--item` on collections is the reverse lookup: given a repo, which collections contain it.

## Social

| Command | What it does |
|---|---|
| `likers <repo>` | The users who liked a repository |
| `likes <name>` | What a user has liked |
| `followers <name>` | A user's followers |
| `following <name>` | Who a user follows |
| `members <name>` | An organization's members |
| `upvoters <ref>` | Who upvoted a paper or a collection |

## Repository contents

| Command | What it does |
|---|---|
| `refs <repo>` | Branches, tags, and conversions |
| `tree <repo> [path]` | The files at a path, `-r` to recurse, `--commits` for per-entry detail |
| `files <repo>` | Every file, flat |
| `commits <repo>` | The commit log, `--rev` to pick a revision |
| `paths <repo> [path...]` | File metadata and security scans for named paths |
| `card <repo>` | The parsed README front matter |
| `readme <repo>` | The card, written to stdout |
| `cat <repo> <path>` | One file, written to stdout |
| `page <ref>` | A rendered page as JSON, 1:1 with what the browser gets |

## Datasets

The dataset viewer is a second API on its own host, and these commands read it.

| Command | What it does |
|---|---|
| `valid <dataset>` | Which viewer capabilities a dataset has |
| `splits <dataset>` | Configs and splits |
| `size <dataset>` | Row and byte counts |
| `schema <dataset>` | Column types for a split |
| `stats <dataset>` | Per-column statistics |
| `rows <dataset>` | Rows, with `--offset`, `-n`, `--order-by` |
| `head <dataset>` | The first rows of a split |
| `dsearch <dataset> <query>` | Full text search within a split |
| `filter <dataset> <where>` | Filter a split with a SQL-ish predicate |
| `parquet <dataset>` | The converted parquet shards |
| `croissant <dataset>` | The hub's own Croissant metadata |

## Graph

| Command | What it does |
|---|---|
| `uri <input>` | Resolve any reference to its canonical `hf://` URI |
| `url <input>` | Resolve any reference to its https location |
| `graph <ref>` | The node and its edges |
| `edges <ref>` | Edges only |
| `children <model>` | The models derived from a model |
| `parents <model>` | The models a model was derived from |
| `citations <paper>` | The repositories tagged with a paper |
| `crawl <ref>` | Walk breadth-first, `--depth`, `--follow`, `--max-nodes`, `--max-requests`, `--nodes-only`, `--edges-only` |
| `rdf <ref>` | Write one entity as RDF, `--format nt\|ttl\|jsonld\|nq` |
| `export <ref>` | Write a whole graph to one file, `--format jsonl\|json\|nt\|ttl\|jsonld\|nq`, `-O` |

## Meta

| Command | What it does |
|---|---|
| `tags [type]` | The controlled vocabulary |
| `tasks` | The curated task pages |
| `sitemap [kind]` | Every id the hub publishes in its sitemaps |
| `dump <kind>` | Fetch every record of one kind |

## Serving

| Command | What it does |
|---|---|
| `serve [--addr]` | Serve the operations over HTTP as NDJSON |
| `mcp` | Run as an MCP server over stdio |
| `version` | Print the version and exit |

## Global flags

These are shared by every operation, so they work the same on every command.

| Flag | Meaning |
|---|---|
| `-o, --output` | Output format: `auto`, `table`, `json`, `jsonl`, `csv`, `tsv`, `url`, `raw` |
| `--fields` | Comma-separated columns to keep |
| `--template` | Go text/template applied per record |
| `--no-header` | Omit the header row in `table` and `csv` |
| `-n, --limit` | Stop after N records (0 means no limit) |
| `-j, --jobs` | Concurrent requests |
| `--token` | Hub token; also read from `$HF_TOKEN`, `$HUGGING_FACE_HUB_TOKEN`, `$HF_HOME/token` |
| `--deep` | Also read the rendered page and merge the fields only it carries |
| `--card` | Include the README body on repo records |
| `--rev` | Branch, tag, or commit sha |
| `--rate` | Minimum delay between requests |
| `--retries` | Retry attempts on rate limit or 5xx |
| `--timeout` | Per-request timeout |
| `--cache` | Response cache directory |
| `--no-cache` | Bypass on-disk caches |
| `--data-dir` | Override the data directory |
| `--db` | Tee every record into a store (e.g. `out.db`, `postgres://...`) |
| `-v, --verbose` | Increase verbosity (repeatable) |
| `-q, --quiet` | Suppress progress output |
| `--color` | `auto`, `always`, or `never` |

See [output formats](/reference/output/) for what `-o`, `--fields`, and `--template` produce, and [configuration](/reference/configuration/) for environment variables and defaults.
