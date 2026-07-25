---
title: "Read a dataset"
description: "Splits, schemas, statistics, and rows, from the dataset viewer."
weight: 30
---

The dataset viewer is a second API on its own host, and it is where the actual
contents of a dataset live. `hf` treats it as part of the same tool: same refs,
same output contract, same flags.

## What a dataset supports

Not every dataset is converted, so start here:

```bash
hf valid rajpurkar/squad
```

It reports five capabilities: `preview`, `viewer`, `search`, `filter`, and
`statistics`. A dataset with `viewer: false` will not answer row requests, and
that is the hub's answer rather than an error on your side.

## Structure

```bash
hf splits rajpurkar/squad     # the configs and their splits
hf size   rajpurkar/squad     # row and byte counts
hf schema rajpurkar/squad --split train
```

A dataset has configs, and each config has splits. Every command below takes
`--config` and `--split`, and defaults to the first config and to `train` when
you leave them off.

## Rows

```bash
hf head rajpurkar/squad -n 5
hf rows rajpurkar/squad --split train --offset 1000 -n 20
hf rows rajpurkar/squad --order-by 'title' -n 10
```

Rows come back as records, so everything the rest of the tool does works:

```bash
hf rows rajpurkar/squad -n 100 -o jsonl | jq -r .row.question
hf rows rajpurkar/squad -n 100 --fields row.title,row.question
```

## Search and filter

```bash
hf dsearch rajpurkar/squad "beyonce" -n 5
hf filter  rajpurkar/squad "title = 'Beyonce'" -n 5
```

`dsearch` is full text over the split. `filter` takes a SQL-ish `WHERE` clause,
which the viewer evaluates server side, so it is the cheap way to pull a subset
out of a dataset far too large to download.

## Statistics

```bash
hf stats rajpurkar/squad --split train
hf stats rajpurkar/squad -o json | jq '.[] | {column_name, column_type, mean}'
```

Per column, the viewer computes a type and a distribution: minimum, maximum,
mean, median, standard deviation, and a histogram for numeric columns, value
counts for categorical ones. This is the fastest way to know what is in a
dataset without pulling a byte of it.

## Parquet

Every converted dataset is republished as parquet shards, and those are what you
want for bulk work:

```bash
hf parquet rajpurkar/squad
hf parquet rajpurkar/squad -o url | xargs -n1 curl -sO
```

## Metadata

```bash
hf dataset rajpurkar/squad          # the hub's record
hf card    rajpurkar/squad          # the parsed README front matter
hf croissant rajpurkar/squad        # the MLCommons document
```

See [linked data](/guides/linked-data/) for what to do with the Croissant
output.
