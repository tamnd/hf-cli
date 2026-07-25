---
title: "Read what the API will not tell you"
description: "The page plane, --deep, and hf page."
weight: 40
---

Parts of the hub have no API route at all, and other parts tell a browser more than they tell `/api`.
The rendered pages carry their data as JSON in `data-target` and `data-props` attribute pairs, which is how the site hydrates itself in the browser.
`hf` reads those pairs rather than scraping text, so what comes back is the site's own structured data, not a guess about its HTML.

## The whole page as JSON

```bash
hf page google-bert/bert-base-uncased
hf page datasets/rajpurkar/squad -o json | jq '.[0].components | keys'
hf page google -o json | jq '.[0].components.OrgProfile'
```

`hf page` is 1:1 with what the browser gets.
The record carries the page's `title`, its `canonical` URL, its `meta` tags, and a `components` map holding every declared component with its props underneath.
If you are trying to work out whether a field exists anywhere, start here.

## Merging it into a record

`--deep` on any read command fetches the page as well as the API and merges the fields only the page carries:

```bash
hf model google-bert/bert-base-uncased --deep
hf dataset rajpurkar/squad --deep
hf org google --deep
```

## What you gain

| With `--deep` | Why the API cannot |
|---|---|
| `tagObjs`, decoded tags with human labels and sub-types | The API returns raw tag strings |
| open and closed discussion counts | No API route |
| linked spaces with their live running state | The API's `spaces` expand is ids only |
| an org's models, datasets, spaces, collections, members, and papers | Seven paginated calls otherwise |
| paper AI summaries, keywords, and upvoters | No API route |
| the dataset viewer's rendered rows and facet counts | Different shape |
| a space's runtime state inline | Saves the separate `/runtime` call |

An org page is the clearest win: one 254 KB payload replaces seven paginated request sequences.

## What it costs

A page is roughly ten times the bytes of the equivalent API call, and it is a second request.
`hf` reaches for it only when asked, and never when a dedicated endpoint returns the same field.
On a crawl of thousands of repos, leaving `--deep` off is usually the right call.

## Commands that are page-only

Some of the surface has no API behind it at all, so these read the page whichever way you ask:

- `hf upvoters` for papers and collections
- `hf post` for a single community post
- `hf blog` for a blog entry's body
- parts of `hf paper`, including the AI summary

They behave like every other command.
The page plane is an implementation detail, and the only place it shows through is the `sources` field on the record, which lists exactly what was fetched to build it.
