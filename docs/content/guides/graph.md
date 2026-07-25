---
title: "Walk the graph"
description: "How hf turns the hub into nodes and edges, and how to crawl them."
weight: 10
---

The hub is already a graph.
A model's card names the datasets it trained on, its tags name the paper it cites, its id names the org that owns it, and the API hands back a hundred space ids that load it.
`hf` reads all of that and gives you nodes and edges.

## Addresses

Every entity has a canonical URI:

```
hf://model/google-bert/bert-base-uncased
hf://dataset/rajpurkar/squad
hf://space/black-forest-labs/FLUX.1-dev
hf://org/google
hf://user/julien-c
hf://collection/fdtn-ai/antares-6a5804c889e78b51c447c38a
hf://paper/1810.04805
hf://tag/arxiv:1810.04805
```

`hf uri` resolves anything you can paste into one of those, and `hf url` goes the other way:

```bash
hf uri https://huggingface.co/datasets/rajpurkar/squad
hf uri datasets/rajpurkar/squad
hf url hf://paper/1810.04805
```

Two segments with no kind prefix is a model, one segment is a namespace.
That is a deliberate bet on what most of the hub is, and `hf get` corrects itself when the guess turns out to be wrong.

## One entity's edges

```bash
hf graph google-bert/bert-base-uncased
hf edges google-bert/bert-base-uncased -o jsonl
```

An edge is a subject, a predicate, an object, and the source it came from:

```json
{"subject":"hf://model/google-bert/bert-base-uncased","predicate":"hf:trainedOn","object":"hf://dataset/bookcorpus","objectKind":"dataset","source":"tag"}
```

`source` is the part worth paying attention to.
It is one of `api`, `page`, `card`, `tag`, or `derived`, and it tells you how much to trust the edge.
An `api` edge is the hub's own record of a relation.
A `card` edge is what an author wrote in their README, which is frequently a name that does not resolve to anything.

## The predicates

Structure:

| Predicate | From | To |
|---|---|---|
| `hf:ownedBy` | any repo | user or org |
| `hf:memberOf` | user | org |
| `hf:contains` | collection | its items |
| `hf:hasFile`, `hf:hasRef`, `hf:hasCommit`, `hf:hasDiscussion` | repo | its contents |
| `hf:hasSplit` | dataset | split |

Lineage:

| Predicate | From | To |
|---|---|---|
| `hf:derivesFrom` | model | model |
| `hf:finetuneOf`, `hf:adapterOf`, `hf:quantizationOf`, `hf:mergeOf` | model | model |
| `hf:trainedOn`, `hf:evaluatedOn` | model | dataset |
| `hf:cites` | repo | paper |

Use:

| Predicate | From | To |
|---|---|---|
| `hf:uses` | space | model or dataset |
| `hf:usedBy` | model or dataset | space |
| `hf:servedBy` | model | inference provider |
| `hf:hasTask`, `hf:hasLibrary`, `hf:hasTag`, `hf:license`, `hf:language` | repo | vocabulary term |

Social: `hf:likes`, `hf:follows`, `hf:upvotes`, `hf:authorOf`, `hf:mentions`, `hf:repliesTo`.

## Crawling

```bash
hf crawl hf://org/google --depth 2 -n 500
hf crawl google-bert/bert-base-uncased --depth 2 --follow ownedBy,derivesFrom
hf crawl hf://org/google --depth 2 --nodes-only -o url
```

The walk is breadth-first, deduplicated by URI, and bounded three ways at once: `--depth`, `--max-nodes`, and `--max-requests`.
Leave the bounds at their defaults for anything exploratory.
Without `--follow`, the walk takes the structural predicates only, which is what keeps a crawl from wandering into every model that shares a license with the seed.

Output streams, so this works on an org with tens of thousands of repos:

```bash
hf crawl hf://org/google --depth 2 -o jsonl > google.jsonl
```

## Lineage shortcuts

Three relations are common enough to have their own commands:

```bash
hf children google-bert/bert-base-uncased    # fine-tunes, adapters, quants, merges
hf parents  Jorgeutd/bert-base-uncased-finetuned-surveyclassification
hf citations 1810.04805                      # every repo tagged with the paper
```

`hf children` is the one that is not obvious from the API docs: it runs `filter=base_model:<relation>:<id>` once per relation and merges the results, which is the only way to get a model's descendants.

## Composing

`-o url` on any of these gives you input for the next command:

```bash
hf children google-bert/bert-base-uncased -n 20 -o url | xargs -n1 hf get
hf crawl hf://org/google --depth 1 --nodes-only -o url | xargs -n1 hf uri
```

Next: [export it as linked data](/guides/linked-data/).
