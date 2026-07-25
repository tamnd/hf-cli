---
title: "Export linked data"
description: "RDF, JSON-LD, and Croissant out of the hub."
weight: 20
---

Once the hub is nodes and edges, serialising it as RDF is a formatting problem.
`hf` writes four RDF formats plus the hub's own Croissant documents.

## One entity

```bash
hf rdf google-bert/bert-base-uncased                  # N-Triples, the default
hf rdf google-bert/bert-base-uncased --format ttl
hf rdf google-bert/bert-base-uncased --format jsonld
hf rdf google-bert/bert-base-uncased --format nq
```

Turtle is the one to read by eye:

```turtle
@prefix hf: <https://huggingface.co/ns#> .
@prefix schema: <https://schema.org/> .

<https://huggingface.co/google-bert/bert-base-uncased>
    a hf:Model ;
    a schema:SoftwareApplication ;
    schema:name "google-bert/bert-base-uncased" ;
    hf:cites <https://huggingface.co/papers/1810.04805> ;
    schema:citation <https://huggingface.co/papers/1810.04805> ;
    hf:downloads "93252036"^^xsd:integer ;
    hf:trainedOn <https://huggingface.co/datasets/bookcorpus> .
```

## The vocabulary

Subjects are the live https URLs, not the `hf://` URIs, so a triple store
dereferences to a page a human can read.

Terms come from `schema.org` where one fits and from an `hf:` namespace where
none does. Where both apply, both are emitted: `hf:cites` and `schema:citation`
carry the same object, so a consumer that only knows schema.org still gets
something and a consumer that knows the hub gets the precise term.

| Prefix | Namespace |
|---|---|
| `hf` | `https://huggingface.co/ns#` |
| `schema` | `https://schema.org/` |
| `cr` | `http://mlcommons.org/croissant/` |
| `dcterms` | `http://purl.org/dc/terms/` |
| `foaf` | `http://xmlns.com/foaf/0.1/` |

Types map like this:

| Kind | Types |
|---|---|
| model | `hf:Model`, `schema:SoftwareApplication` |
| dataset | `hf:Dataset`, `schema:Dataset`, `cr:Dataset` |
| space | `hf:Space`, `schema:WebApplication` |
| user | `hf:User`, `schema:Person`, `foaf:Person` |
| org | `hf:Organization`, `schema:Organization` |
| paper | `hf:Paper`, `schema:ScholarlyArticle` |

Literals are typed: counts as `xsd:integer`, timestamps as `xsd:dateTime`,
booleans as `xsd:boolean`.

## N-Quads and provenance

N-Quads puts the fetch URL in the fourth position, so the triple store can
answer which page told you a thing:

```bash
hf rdf google-bert/bert-base-uncased --format nq | head -1
```

## A whole graph

`hf export` runs a crawl and writes the result as one file:

```bash
hf export hf://org/google --depth 2 --format ttl -O google.ttl
hf export google-bert/bert-base-uncased --depth 1 --format jsonl
hf export hf://org/google --depth 2 --format json
```

`jsonl` is the format to reach for on anything large. It writes nodes first and
edges after, so a reader building an index in one pass never meets an edge
before both of its ends. N-Triples and N-Quads stream too. Turtle, JSON-LD, and
`json` need the whole graph in memory, which is fine for one entity and a bad
idea for an org.

## Croissant

The hub publishes an MLCommons Croissant document for every dataset, and it is
already correct JSON-LD over schema.org. `hf` passes it through rather than
reimplementing it:

```bash
hf croissant rajpurkar/squad
```

The dataset's own graph is folded in under `hf:graph`, so you get the standard
document plus everything `hf` knows that Croissant has no place for.

## Loading it

```bash
hf export hf://org/google --depth 2 --format nt -O google.nt
# then load google.nt into whatever store you use
```

Nothing here needs a triple store. `hf export --format jsonl | jq` answers most
questions people actually ask, and the RDF is there for when it does not.
