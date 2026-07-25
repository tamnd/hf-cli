---
title: "Output formats"
description: "The output contract every command shares: formats, columns, and templates."
weight: 30
---

Every command renders through one formatter, so the same flags work everywhere.
Pick a format with `-o`, or let `hf` choose: a table when it is writing to a terminal, JSONL when it is writing to a pipe.

## Formats

```bash
hf models --author google -o table      # a bordered grid for reading
hf models --author google -o list       # one section per record, good for detail
hf models --author google -o markdown   # a GitHub table to paste somewhere
hf models --author google -o jsonl      # one JSON object per line
hf models --author google -o json       # a single JSON array
hf models --author google -o csv        # spreadsheet friendly
hf models --author google -o tsv        # tab separated
hf models --author google -o url        # the hub URL of each record
hf cat google-bert/bert-base-uncased -o raw   # the bytes, unformatted
```

| Format | Best for |
|---|---|
| `table` | Reading a list on a terminal |
| `list` | Reading one record, where a grid would be too wide |
| `markdown` | Pasting into a doc, an issue, or a README |
| `jsonl` | Piping into another tool, one object at a time |
| `json` | Loading a whole result as an array |
| `csv` / `tsv` | Spreadsheets and quick column math |
| `url` | Feeding hub URLs into other commands |
| `raw` | Response bodies and file contents |

## Columns are a summary, JSON is everything

A record models everything the hub said about a thing, which for a model is around sixty fields and for an org page is more.
That does not fit across a screen, so each record type marks the handful of fields worth a column and hides the rest:

```bash
hf models --author google -o csv | head -1
id,gated,likes,downloads,modified,task,library
```

Hidden means hidden from `table`, `list`, `markdown`, `csv`, and `tsv` only.
The JSON formats always carry the whole record, so a field you cannot see in the table is still one field away in a pipe:

```bash
hf model google-bert/bert-base-uncased -o json | jq '.[0].safetensors'
```

## Narrowing and widening columns

`--fields` names exactly the columns you want, in the order you want them, and a name can be one of the hidden fields:

```bash
hf models --author google --fields id,downloads
hf models --author google --fields id,trendingScore,createdAt
```

The names are the JSON keys.
A name the record does not have comes back as an empty column, so a typo shows up as a blank rather than shifting the row.

`--no-header` drops the header row, which helps when a downstream tool expects bare rows.

## Templating rows

For full control over each line, apply a Go text/template.
The template runs against the record's JSON, so the names are the JSON keys, lowercase, the same ones `jq` would use:

```bash
hf models --author google --template '{{.id}} has {{.downloads}} downloads'
hf datasets -n 5 --template '{{.id}}: {{.likes}} likes'
```

A name the record does not carry prints `<no value>` rather than failing, which is usually a JSON key spelled as a Go field name.

## Why auto-detection helps

Because the default adapts to the destination, the same command reads well by hand and parses cleanly in a pipe:

```bash
hf models --author google       # a table, because this is a terminal
hf models --author google | jq  # JSONL, because this is a pipe
```

You only reach for `-o` when you want something other than that default.
