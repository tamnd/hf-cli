---
title: "hf"
description: "Read huggingface.co as data."
heroTitle: "The Hugging Face hub, as records"
heroLead: "Every model, dataset, space, user, org, collection, paper, and post the hub publishes, with every field its source returned, a canonical hf:// address, and edges to everything it names."
heroPrimaryURL: "/getting-started/quick-start/"
heroPrimaryText: "Get started"
---

`hf` is one pure-Go binary.
It reads huggingface.co over plain HTTPS, shapes what comes back into typed records, and prints output that pipes into the rest of your tools.
Public data needs no API key.

```bash
hf model google-bert/bert-base-uncased      # every field the hub returns
hf models --author google --sort downloads  # list a million, stop when you like
hf graph google-bert/bert-base-uncased      # the node and its edges
hf rdf   google-bert/bert-base-uncased --format ttl
```

Output adapts to where it goes: an aligned table on your terminal, JSONL the moment you pipe it somewhere.

![hf reading a model, listing an org, showing a dataset schema, printing graph edges, and emitting schema.org triples](/demo.gif)

## What makes it different

- **Nothing is dropped.** Every record type sweeps the fields it does not model into an `extra` map, and the test suite fails when one shows up.
  If the hub returns it, you get it.
- **Two planes.** Parts of the hub have no API, and other parts tell a browser more than they tell `/api`.
  `--deep` reads the rendered page and merges the JSON it already carries.
- **It is a graph.** Repo ids, tags, card front matter, and explicit id references all become typed edges, each one recording where it came from.
- **It exports linked data.** N-Triples, Turtle, JSON-LD, N-Quads, and the hub's own Croissant document.

## Two ways to use it

- **As a command** for reading the hub by hand or in a script.
  Start with the [quick start](/getting-started/quick-start/).
- **As a resource-URI driver** so a host like [ant](https://github.com/tamnd/ant) can address the hub as `hf://` URIs and follow links across sites.
  See [resource URIs](/guides/resource-uris/).

Both are the same code: one operation, declared once, is a CLI command, an HTTP route, an MCP tool, and a URI dereference.

## Where to go next

- New here?
  Read the [introduction](/getting-started/introduction/), then the [quick start](/getting-started/quick-start/).
- Installing?
  See [installation](/getting-started/installation/).
- Doing a specific job?
  The [guides](/guides/) are task-first.
- Need every flag?
  The [CLI reference](/reference/cli/) is the full surface.
