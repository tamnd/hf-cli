---
title: "Resource URIs"
description: "Use hf as a database/sql-style driver so a host program can address the hub as hf:// URIs."
weight: 50
---

`hf` is a command line, but the `hf` Go package is also a driver that makes
huggingface.co addressable as resource URIs. A host program registers it the way
a program registers a database driver with `database/sql`, then dereferences
`hf://` URIs without knowing anything about how the hub is fetched.

The host that does this today is [ant](https://github.com/tamnd/ant), a single
binary that puts one URI namespace over a family of site tools. The examples
below use `ant`; any program that links the package gets the same behaviour.

## Mounting the driver

A host enables the driver with one blank import, exactly like `import _
"github.com/lib/pq"`:

```go
import _ "github.com/tamnd/hf-cli/hf"
```

The package's `init` registers a domain with the scheme `hf` for the host
`huggingface.co`. The standalone `hf` binary does not change.

## The addressable types

A URI is `scheme://authority/id`, and the authority is the kind:

| URI | What it is |
|---|---|
| `hf://model/google-bert/bert-base-uncased` | a model |
| `hf://dataset/rajpurkar/squad` | a dataset |
| `hf://space/black-forest-labs/FLUX.1-dev` | a space |
| `hf://kernel/kernels-community/activation` | a compute kernel |
| `hf://user/julien-c` | a user |
| `hf://org/google` | an organization |
| `hf://namespace/huggingface` | a name that is one of the two, before you know which |
| `hf://collection/fdtn-ai/antares-6a5804c889e78b51c447c38a` | a collection |
| `hf://paper/1810.04805` | a paper |
| `hf://post/PeetPedro/340848718906662` | a community post |
| `hf://blog/gradio-mcp` | a blog entry |
| `hf://discussion/model/google-bert/bert-base-uncased#1` | a discussion or pull request |
| `hf://tag/arxiv:1810.04805` | a vocabulary term |
| `hf://task/text-generation` | a curated task page |
| `hf://file/model/google-bert/bert-base-uncased@main/config.json` | a file at a revision |
| `hf://ref/model/google-bert/bert-base-uncased@main` | a branch, tag, or sha |
| `hf://split/rajpurkar/squad/plain_text/train` | a dataset split |

The last few look busier than the rest because they have to carry the repo's
kind and its revision. A file is only identified once you know which of the four
repo kinds owns it and which revision you meant, and an id that leaves either
out would not round trip.

```bash
ant get hf://model/google-bert/bert-base-uncased    # the record
ant cat hf://model/google-bert/bert-base-uncased    # the README body
ant url hf://dataset/rajpurkar/squad                # the live https URL
ant resolve https://huggingface.co/google/gemma-3-27b-it   # a pasted link, back to its URI
```

The same resolution works in the binary without a host:

```bash
hf uri https://huggingface.co/datasets/rajpurkar/squad
hf url hf://paper/1810.04805
```

## Walking the graph

`ls` lists the members of a collection, and every member is itself an
addressable URI, so a host can follow the graph and write it to disk:

```bash
ant ls     hf://org/google              # the org's repos, each addressable
ant ls     hf://dataset/rajpurkar/squad # its splits
ant export hf://org/google --follow 1 --to ./data
```

Because the records carry typed edges, `ant export --follow` and `ant graph`
walk those edges too, and across tools when an edge points at another site's
scheme. An `arxiv:` tag on a model is a citation edge to a paper, and a host
that also mounts an arXiv driver can follow it off the hub entirely.

## Why this is the same code

The driver and the binary share one definition per operation. A resolver op
answers both `hf model` on the command line and `ant get hf://model/...` through
a host, from the same handler and the same client. There is no second
implementation to keep in step.
