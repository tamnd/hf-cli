package hf

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// decode.go is the machinery behind one rule from the spec: a record has every
// field its source returned. The hub adds fields without warning, and a decoder
// that silently drops what it does not recognise turns that into invisible data
// loss. So every record type decodes through decodeExtra, which fills the typed
// fields and sweeps everything else into Extra.

// decodeExtra unmarshals b into v, then collects every top-level key that did
// not map to a field of v into extra. v must be a pointer to a struct, and the
// caller passes a defined type whose method set lacks UnmarshalJSON, so this
// does not recurse.
//
// The optional drop list names keys that carry nothing new: the hub repeats a
// repo id as modelId on model rows, and sends an always-empty key on dataset
// rows. Sweeping those into Extra would report drift that is not there, and
// Extra is only useful if everything in it is worth reading.
func decodeExtra(b []byte, v any, extra *map[string]json.RawMessage, drop ...string) error {
	if err := json.Unmarshal(b, v); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(b, &all); err != nil {
		// A non-object body has no stray keys to collect; the typed decode above
		// already reported anything that mattered.
		return nil //nolint:nilerr
	}
	known := knownNames(reflect.TypeOf(v))
	var rest map[string]json.RawMessage
	for k, raw := range all {
		if known[k] || slices.Contains(drop, k) {
			continue
		}
		if rest == nil {
			rest = map[string]json.RawMessage{}
		}
		rest[k] = raw
	}
	if rest != nil {
		*extra = rest
	}
	return nil
}

var namesCache sync.Map // reflect.Type -> map[string]bool

// knownNames returns the set of JSON keys a struct type consumes, following
// embedded structs the way encoding/json does. It is cached because a crawl
// decodes the same types hundreds of thousands of times.
func knownNames(t reflect.Type) map[string]bool {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if v, ok := namesCache.Load(t); ok {
		return v.(map[string]bool)
	}
	out := map[string]bool{}
	collectNames(t, out)
	namesCache.Store(t, out)
	return out
}

func collectNames(t reflect.Type, out map[string]bool) {
	if t.Kind() != reflect.Struct {
		return
	}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			collectNames(ft, out)
			continue
		}
		if name == "" {
			name = f.Name
		}
		out[name] = true
	}
}

// alias is the marker every UnmarshalJSON uses:
//
//	func (m *Model) UnmarshalJSON(b []byte) error {
//		type raw Model
//		return decodeExtra(b, (*raw)(m), &m.Extra)
//	}
//
// The local defined type drops the method set, which is what stops the decode
// from calling itself.

// --- small decode helpers used by the hand-written unmarshalers ---

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func trimSpace(b []byte) []byte { return bytes.TrimSpace(b) }

// stringOf renders a decoded JSON scalar as a string. Card front matter is
// author-written, so a field that should hold a string routinely holds a number
// or a bool, and refusing those would lose real values.
func stringOf(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(x), true
	case json.Number:
		return x.String(), true
	case nil:
		return "", false
	default:
		return "", false
	}
}

// Time is a timestamp that tolerates the several shapes the hub sends: RFC3339
// with or without fractional seconds, and an epoch in seconds or milliseconds.
type Time struct{ time.Time }

// UnmarshalJSON parses the timestamp forms the hub uses.
func (t *Time) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), `"`)
	if s == "" || s == "null" {
		return nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 { // milliseconds
			t.Time = time.UnixMilli(n).UTC()
		} else {
			t.Time = time.Unix(n, 0).UTC()
		}
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z", "2006-01-02"} {
		if v, err := time.Parse(layout, s); err == nil {
			t.Time = v.UTC()
			return nil
		}
	}
	return nil
}

// MarshalJSON writes RFC3339, or null for the zero time.
func (t Time) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return json.Marshal(t.UTC().Format(time.RFC3339))
}
