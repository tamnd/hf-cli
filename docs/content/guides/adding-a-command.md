---
title: "Add a command"
description: "Declare an operation once and get a command, a route, an MCP tool, and a URI type."
weight: 60
---

Everything `hf` does is one `kit.Handle` registration in `hf/ops.go`, backed by
a client method and a record type. Add those three pieces and every surface
updates itself.

## 1. The record

Records live in the `hf` package, one file per family. Every one embeds `Meta`,
which carries the URI, the live URL, the sources it was built from, and the
`Extra` map:

```go
type Widget struct {
    Meta
    ID    string `json:"id"`
    Title string `json:"title,omitempty"`
    Owner string `json:"owner,omitempty"`

    Extra map[string]json.RawMessage `json:"-"`
}

func (w *Widget) UnmarshalJSON(b []byte) error {
    type raw Widget
    return decodeExtra(b, (*raw)(w), &w.Extra)
}
```

`decodeExtra` is the rule the whole tool is built on: decode what you model,
keep what you do not. Without it, a field the hub adds next month is a field
nobody ever sees. With it, the test suite fails and you go and model it.

## 2. The client method

```go
func (c *Client) Widget(ctx context.Context, id string) (*Widget, error) {
    resp, err := c.Get(ctx, c.api("widgets", id))
    if err != nil {
        return nil, err
    }
    var w Widget
    if err := json.Unmarshal(resp.Body, &w); err != nil {
        return nil, err
    }
    w.setMeta(KindWidget, w.ID, resp.URL)
    return &w, nil
}
```

`c.Get` handles pacing, auth, retries, caching, and the status-to-error mapping,
so a method that uses it inherits the whole policy. `setMeta` fills the URI, the
URL, and the source list.

## 3. The operation

An input struct declares the arguments and flags by reflection, and the handler
emits records:

```go
type widgetIn struct {
    C   *Client `kit:"inject"`
    Ref string  `kit:"arg" help:"a widget id, a hub URL, or an hf:// URI"`
}

func getWidget(ctx context.Context, in widgetIn, emit func(*Widget) error) error {
    id, err := ResolveRef(KindWidget, in.Ref)
    if err != nil {
        return err
    }
    w, err := in.C.Widget(ctx, id)
    if err != nil {
        return err
    }
    return emit(w)
}
```

Then register it:

```go
kit.Handle(app, kit.OpMeta{
    Name: "widget", Group: "read", Single: true, URIType: KindWidget, Resolver: true,
    Summary: "Read one widget",
    Args:    []kit.Arg{{Name: "ref", Help: "widget id, URL, or hf:// URI"}},
}, getWidget)
```

That is the whole change. The operation is now:

```bash
hf widget <id>                            # the command
curl 'localhost:7777/v1/widget/<id>'      # the route, under serve
ant get hf://widget/<id>                  # the URI dereference, via a host
```

## Input struct tags

| Tag | Meaning |
|---|---|
| `kit:"arg"` | a positional argument |
| `kit:"arg,variadic"` | the rest of the positionals |
| `kit:"flag"` | a flag, named from the field in snake case |
| `kit:"flag,name=order-by"` | a flag with the name spelled out |
| `kit:"flag,short=r"` | a flag with a short form |
| `kit:"flag,inherit"` | a flag the app already defines globally, like `Limit` |
| `kit:"inject"` | filled in by the app, which is how the client arrives |

Alongside them, `help:"..."`, `default:"..."`, and `enum:"a,b,c"` do what they
look like.

## Resolver ops and list ops

Two fields shape how a host treats an operation:

- **`Single: true`** with **`Resolver: true`** marks the canonical one-record
  fetch for a `URIType`. It answers `ant get`.
- **`List: true`** marks a member-lister for a parent resource. It answers
  `ant ls`, and should emit records that are themselves addressable so every
  member is a URI a host can follow.

## Errors

Return the kinds from `kit/errs` and every surface reports the same outcome with
the same exit code:

```go
return errs.Usage("a post id is user/slug, got %q", id)
return errs.NotFound("no such widget: %s", id)
```

The client already maps hub status codes to those kinds, so most handlers just
pass the error up.

## Add it to the tests

`hf/scenario_test.go` has one table that every test drives. Add a line:

```go
{Name: "widget", Run: op(widgetIn{Ref: fixWidget}, getWidget)},
```

Then record the fixture and write the golden:

```bash
make fixtures
```

The offline run now covers the new command: it checks that it returns records,
that nothing landed in `Extra`, and that the set of fields it produces does not
shrink later. See the repository README for how the fixtures work.

## Commands that write bytes

An operation that produces a file rather than records is not a `kit.Handle`. Use
`kit.Command` and register it in `cli/`, the way `cat`, `readme`, `rdf`,
`export`, and `croissant` do. Those are the escape hatch, and there are only a
handful of them for a reason: a record is almost always the better answer.
