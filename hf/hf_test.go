package hf

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tamnd/any-cli/kit/render"
	"github.com/tamnd/hf-cli/hftest"
)

// hf_test.go drives the scenario table two ways. TestRecord fetches from the
// live hub and writes fixtures; everything else replays those fixtures and
// makes assertions no live run could make reliably.
//
// The assertions are the three things that actually matter for a tool whose job
// is to lose nothing:
//
//   - every command still returns records rather than an error,
//   - no record carries an unmodelled field, which is what Extra means,
//   - the set of fields a command produces has not shrunk, which is what the
//     shape goldens pin down.
//
// Values are deliberately not pinned. A download count changes hourly and a
// golden full of them would be re-recorded so often that nobody would read the
// diff, which is how a golden stops being a test.

var update = flag.Bool("update", false, "rewrite the shape goldens")

const (
	fixtureDir = "testdata/live"
	shapeDir   = "testdata/shape"
)

// newTestClient builds the client the scenarios run through. Retries are off
// because in replay a failure is a missing fixture and repeating it only adds
// backoff, and the rate limiter is off because nothing is being asked of a real
// server.
func newTestClient(rt http.RoundTripper, s scenario) *Client {
	c := NewClient()
	c.HTTP = &http.Client{Transport: rt, Timeout: 60 * time.Second}
	c.Token = ""
	c.Rate = 0
	c.Retries = 0
	c.CacheDir = ""
	c.NoCache = true
	c.Workers = 1
	c.Deep = s.Deep
	c.Card = s.Card
	return c
}

// TestRecord refreshes the fixture set from the live hub. It is skipped unless
// HF_RECORD is set, so an ordinary test run never touches the network, and it
// records anonymously so the fixtures are what any reader sees and no private
// data can end up in the repository.
func TestRecord(t *testing.T) {
	if os.Getenv("HF_RECORD") == "" {
		t.Skip("set HF_RECORD=1 to re-record fixtures, or run: make fixtures")
	}
	rec, err := hftest.NewRecorder(fixtureDir, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, s := range scenarios() {
		out, err := s.Run(ctx, newTestClient(rec, s))
		switch {
		case err != nil:
			t.Errorf("%s: %v", s.Name, err)
		case len(out) == 0:
			t.Errorf("%s: returned nothing", s.Name)
		default:
			t.Logf("%-18s %d records", s.Name, len(out))
		}
	}
	t.Logf("recorded %d exchanges into %s", rec.Count(), fixtureDir)
}

// TestScenarios replays every command in the table.
func TestScenarios(t *testing.T) {
	rp := replayer(t)
	for _, s := range scenarios() {
		t.Run(s.Name, func(t *testing.T) {
			out, err := s.Run(context.Background(), newTestClient(rp, s))
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			if len(out) == 0 {
				t.Fatal("returned no records")
			}
			for _, path := range unmodelled(out) {
				t.Errorf("unmodelled field %s: the hub returns it and hf drops it into Extra", path)
			}
			checkShape(t, s.Name, out)
		})
	}
	if missing := rp.Misses(); len(missing) > 0 {
		t.Errorf("no fixture for %d URLs:\n  %s", len(missing), strings.Join(missing, "\n  "))
	}
}

// TestFixturesAreUsed fails when the fixture set has grown things nothing asks
// for any more. Left alone, a stale fixture makes the set look like it covers
// more than the tests do.
func TestFixturesAreUsed(t *testing.T) {
	rp := replayer(t)
	ctx := context.Background()
	for _, s := range scenarios() {
		if _, err := s.Run(ctx, newTestClient(rp, s)); err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
	}
	if unused := rp.Unused(); len(unused) > 0 {
		t.Errorf("%d fixtures nothing asked for:\n  %s", len(unused), strings.Join(unused, "\n  "))
	}
}

func replayer(t *testing.T) *hftest.Replayer {
	t.Helper()
	rp, err := hftest.NewReplayer(fixtureDir)
	if err != nil {
		t.Fatalf("%v\nrecord the fixtures first: make fixtures", err)
	}
	return rp
}

// --- the unmodelled-field check ---

// unmodelled finds every non-empty Extra map in a result and reports where it
// was. It walks the Go values rather than the JSON output because several Extra
// maps are marked json:"-" and would be invisible to anything reading only what
// the command prints.
func unmodelled(out []any) []string {
	found := map[string]bool{}
	for _, rec := range out {
		walkExtra(reflect.ValueOf(rec), "", found, 0)
	}
	keys := make([]string, 0, len(found))
	for k := range found {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// maxWalk stops a walk that has clearly gone wrong. Records are trees, so
// nothing real goes anywhere near this deep.
const maxWalk = 24

func walkExtra(v reflect.Value, path string, found map[string]bool, depth int) {
	if depth > maxWalk || !v.IsValid() {
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			walkExtra(v.Elem(), path, found, depth+1)
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			walkExtra(v.Index(i), path+"[]", found, depth+1)
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			walkExtra(v.MapIndex(k), path+"{}", found, depth+1)
		}
	case reflect.Struct:
		t := v.Type()
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := path + "." + f.Name
			if f.Anonymous {
				name = path
			}
			if f.Name == "Extra" && v.Field(i).Kind() == reflect.Map {
				for _, k := range v.Field(i).MapKeys() {
					found[strings.TrimPrefix(name+"."+k.String(), ".")] = true
				}
				continue
			}
			walkExtra(v.Field(i), name, found, depth+1)
		}
	}
}

// --- the shape goldens ---

// checkShape compares the set of populated field paths against the recorded
// one. A path that disappears means a decoder stopped filling something it used
// to fill, which is the regression this whole package exists to prevent, and a
// path that appears is new coverage worth seeing in a diff.
func checkShape(t *testing.T, name string, out []any) {
	t.Helper()
	got := shapeOf(out)
	path := filepath.Join(shapeDir, name+".txt")
	if *update {
		if err := os.MkdirAll(shapeDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nwrite the goldens first: go test ./hf -update", err)
	}
	want := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for _, line := range diff(want, got) {
		t.Error(line)
	}
}

// shapeOf renders what a result contains: one line per distinct field path,
// with the JSON type at the leaf. It goes through the marshaller because that
// is what a caller of hf actually receives, so a field this misses is a field
// nobody downstream can see either.
func shapeOf(out []any) []string {
	paths := map[string]string{}
	for _, rec := range out {
		// A scenario that returns bytes has no field paths, so its shape is its
		// size. The fixture behind it is frozen, so that number is stable, and it
		// is what catches a read that came back with a redirect or a pointer file
		// instead of the content.
		if s, ok := rec.(string); ok {
			paths[fmt.Sprintf("(bytes) %d", len(s))] = ""
			continue
		}
		b, err := json.Marshal(rec)
		if err != nil {
			paths["!marshal "+err.Error()] = ""
			continue
		}
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			// A record that is not JSON at all is still a shape: the byte-producing
			// scenarios return a plain string.
			paths[fmt.Sprintf("%T", rec)] = ""
			continue
		}
		walkShape(v, "", paths)
	}
	lines := make([]string, 0, len(paths))
	for p, kind := range paths {
		if kind == "" {
			lines = append(lines, p)
			continue
		}
		lines = append(lines, p+" "+kind)
	}
	sort.Strings(lines)
	return lines
}

func walkShape(v any, path string, paths map[string]string) {
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			paths[path] = "object"
			return
		}
		for k, sub := range x {
			key := k
			if path != "" {
				key = path + "." + k
			}
			walkShape(sub, key, paths)
		}
	case []any:
		if len(x) == 0 {
			paths[path+"[]"] = "empty"
			return
		}
		for _, sub := range x {
			walkShape(sub, path+"[]", paths)
		}
	case string:
		paths[path] = "string"
	case float64:
		paths[path] = "number"
	case bool:
		paths[path] = "bool"
	case nil:
		paths[path] = "null"
	}
}

// diff reports what a golden gained and lost, which is more useful than a
// wall-of-text mismatch when a shape file runs to a few hundred lines.
func diff(want, got []string) []string {
	in := func(list []string) map[string]bool {
		m := make(map[string]bool, len(list))
		for _, s := range list {
			m[s] = true
		}
		return m
	}
	haveWant, haveGot := in(want), in(got)
	var out []string
	for _, s := range want {
		if !haveGot[s] {
			out = append(out, "lost:  "+s)
		}
	}
	for _, s := range got {
		if !haveWant[s] {
			out = append(out, "added: "+s)
		}
	}
	if len(out) > 0 {
		out = append(out, "if this is the intended change, refresh with: go test ./hf -update")
	}
	return out
}

// --- the table columns ---

// TestTableColumns pins what each command shows on a terminal. The record types
// model everything the hub said, which is far more than fits across a screen,
// so every type marks the few fields worth a column and hides the rest. Nothing
// else checks that: a type that forgets to says so only by printing a wall of
// raw JSON, and it prints it in the default format, which is the one a person
// sees first.
func TestTableColumns(t *testing.T) {
	rp := replayer(t)
	var lines []string
	for _, s := range scenarios() {
		out, err := s.Run(context.Background(), newTestClient(rp, s))
		if err != nil {
			t.Fatalf("%s: %v", s.Name, err)
		}
		cols := columnsOf(out[0])
		if cols == "" {
			continue // a byte-producing scenario has no columns
		}
		for _, c := range strings.Split(cols, ",") {
			if len(c) > 24 {
				t.Errorf("%s: column %q is too wide to be a column", s.Name, c)
			}
		}
		lines = append(lines, s.Name+": "+cols)
	}
	sort.Strings(lines)
	checkGolden(t, "columns", lines)
}

// columnsOf renders one record's header row through the same renderer the CLI
// uses, so the answer is what a person would see rather than what the tags say.
func columnsOf(rec any) string {
	var buf strings.Builder
	r, err := render.New(render.Options{Format: render.CSV, Writer: &buf})
	if err != nil {
		return ""
	}
	if err := r.Emit(rec); err != nil {
		return ""
	}
	if err := r.Flush(); err != nil {
		return ""
	}
	header, _, _ := strings.Cut(buf.String(), "\n")
	return header
}

// checkGolden compares a set of lines against testdata/<name>.txt.
func checkGolden(t *testing.T, name string, got []string) {
	t.Helper()
	path := filepath.Join("testdata", name+".txt")
	if *update {
		if err := os.WriteFile(path, []byte(strings.Join(got, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\nwrite the goldens first: go test ./hf -update", err)
	}
	want := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	for _, line := range diff(want, got) {
		t.Error(line)
	}
}
