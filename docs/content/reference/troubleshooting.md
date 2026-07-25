---
title: "Troubleshooting"
description: "The handful of things that trip people up, and how to fix each one."
weight: 40
---

Most of what goes wrong here is the hub telling you something rather than `hf`
misbehaving. The exit code says which.

## Exit codes

Every surface reports the same outcome the same way, so a script can branch on
the code without reading the message:

| Code | Meaning |
|---|---|
| 0 | it worked |
| 1 | something unclassified went wrong |
| 2 | you asked for something that is not a valid request |
| 3 | the request was fine and there was nothing to return |
| 4 | the hub wants a token |
| 5 | rate limited |
| 6 | no such thing |
| 7 | the hub does not offer this |
| 8 | the network failed |

3 and 6 are worth keeping apart. `hf models --author nobody` exits 3 because
searching for an author with no models is a legitimate empty answer, while
`hf model nobody/nothing` exits 6 because that repo does not exist.

## Requests start failing or returning 429

The hub rate limits like any public site. `hf` already paces requests, retries
429 and 5xx with backoff, and honours `Retry-After`, so a hard limit getting
through means the defaults are too eager for what you are doing. Raise the delay
with `--rate 1s`, drop `--jobs` to 1 or 2, and run it again. Anonymous requests
get a smaller budget than authenticated ones, so a token helps here even for
public data.

## A repo you can see in the browser comes back as not found

Two different things produce this.

If you are logged in on the web and anonymous on the command line, a private or
gated repo is invisible to `hf`, and the hub reports it as not found rather than
admitting it exists. Set `HF_TOKEN` or pass `--token` and try again.

If you are already authenticated, check the name. A dataset id and a model id
can collide, and a bare `squad` resolves to whichever kind the command wants.
Spell it out as `hf://dataset/rajpurkar/squad` or use `hf uri` on the browser URL
to see what it resolves to.

## A gated dataset reports that it does not exist

`hf valid`, `hf splits`, and the rest of the viewer commands go through the
dataset server, which answers gated and private datasets with a single message
covering both, something like "does not exist, or is not accessible with the
current credentials". That is an auth answer wearing a not-found coat. Supply a
token that has accepted the dataset's terms.

## The dataset viewer says the dataset is not valid

```bash
hf valid rajpurkar/squad
```

`valid: false` means the hub has not indexed that dataset into the viewer, not
that the dataset is broken. Splits, schema, rows, search, filter, and statistics
all depend on that index, so they will be empty too. The repo files are still
readable with `hf files` and `hf cat`.

## A field you can see on the page is missing from the record

The API and the rendered page are two different data planes and the page usually
carries more. Add `--deep` to merge in what only the page has:

```bash
hf model google-bert/bert-base-uncased --deep
```

If it is still missing, dump the whole page and look for it directly:

```bash
hf page google-bert/bert-base-uncased -o json | jq '.[0].components | keys'
```

Anything that turns up there and not in the record is a gap worth reporting, and
anything that landed in the record's `Extra` map is a field nobody has modelled
yet. See [the page plane](/guides/pages/).

## A crawl runs longer than you expected

`hf crawl` is bounded three ways and all three have to be set for it to stop
where you want: `--depth` for how far it follows edges, `--max-nodes` for how
much it will collect, and `-n` for how many records it emits. Leaving
`--max-nodes` at its default of 10000 on a hub-wide start is a long afternoon.
Adding `--deep` to a crawl multiplies the byte count by roughly ten.

## Answers look stale

Responses are cached on disk for 15 minutes, longer for things that cannot
change. Pass `--no-cache` for one fresh run. See
[configuration](/reference/configuration/) for the lifetimes.

## The binary is not on your PATH

`go install` puts the binary in `$(go env GOPATH)/bin`, usually `~/go/bin`, and a
release archive leaves it wherever you unpacked it. Add that directory to your
`PATH`. See [installation](/getting-started/installation/).

## Seeing what hf actually did

`-v` logs every request with its URL, its status, and whether it came from the
cache. That is normally enough to tell a rate limit apart from an empty result,
or a cache hit apart from a fetch. Every record also carries a `sources` field
listing what was fetched to build it.
