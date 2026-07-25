---
title: "Configuration"
description: "Tokens, environment variables, defaults, and the cache."
weight: 20
---

`hf` needs almost no configuration: public data reads anonymously out of the
box. The settings below cover authentication, politeness, and storage.

## Tokens

Public reads need no token. A token gets you private and gated repositories,
your own drafts, and `hf whoami`. `hf` looks for one the way the official tools
do:

1. `--token`
2. `$HF_TOKEN`
3. `$HUGGING_FACE_HUB_TOKEN`
4. `$HF_HOME/token`, defaulting to `~/.cache/huggingface/token`, which is what
   `huggingface-cli login` writes

If you already log in with the official CLI, `hf` picks that token up with no
further setup.

Be aware of what a token changes. An authenticated read can return fields an
anonymous one does not, so a record fetched with a token is not necessarily the
record another person sees.

## Defaults

| Setting | Default | Flag |
|---|---|---|
| Delay between requests | 150ms | `--rate` |
| Retries on 429 and 5xx | 5, with backoff and `Retry-After` honoured | `--retries` |
| Per-request timeout | 30s | `--timeout` |
| Concurrent requests | 8 | `-j, --jobs` |
| Cache lifetime | 15 minutes | `--no-cache` to bypass |

`--jobs` and `--rate` multiply the way you would want: eight workers at 150ms is
about 6.6 requests a second in aggregate, not 53.

## The cache

Responses are cached on disk, keyed on the full URL plus whether the request was
authenticated, because an authenticated response can differ. Lifetimes vary by
what was fetched:

| What | Lifetime |
|---|---|
| Anything pinned to a full commit sha | forever, since it cannot change |
| The tag taxonomy and the task pages | 24 hours |
| Everything else | 15 minutes |

`--cache` picks the directory and `--no-cache` bypasses it for one run.

## The data directory

Caches and any record store live under one data directory, chosen in this order:

1. `--data-dir`
2. `HF_DATA_DIR`
3. `$XDG_DATA_HOME/hf`
4. `~/.local/share/hf`

## Environment variables

Every flag has an environment fallback, prefixed `HF_` in upper case with dashes
as underscores:

```bash
export HF_RATE=1s        # same as --rate 1s
export HF_DATA_DIR=~/data/hf
```

Flags win over environment variables, which win over the built-in defaults.

## Sending records to a store

`--db` tees every emitted record into a store as a side effect of reading, so a
session fills a local database without a separate import step:

```bash
hf models --author google -n 1000 --db out.db
hf crawl hf://org/google --depth 2 --db 'postgres://...'
```

On a crawl it doubles as resumability: the store remembers what you already
fetched.
