package hf

import (
	"encoding/json"
	"testing"
	"time"
)

// The functions in here take a string and return a string. They need no network
// and no fixture, and they are where a wrong answer is least visible: a
// misclassified reference does not fail, it fetches the wrong thing.

func TestClassify(t *testing.T) {
	cases := []struct {
		in   string
		kind string
		id   string
	}{
		// Bare ids, and the two default rules.
		{"google-bert/bert-base-uncased", KindModel, "google-bert/bert-base-uncased"},
		{"google", KindNamespace, "google"},
		{"@julien-c", KindNamespace, "julien-c"},

		// hf:// URIs round trip whatever they say.
		{"hf://dataset/squad", KindDataset, "squad"},
		{"hf://org/google", KindOrg, "google"},

		// A path with the site's own prefix on the front.
		{"datasets/squad", KindDataset, "squad"},
		{"spaces/stabilityai/stable-diffusion", KindSpace, "stabilityai/stable-diffusion"},

		// Hub URLs, in the forms people paste.
		{"https://huggingface.co/google-bert/bert-base-uncased", KindModel, "google-bert/bert-base-uncased"},
		{"https://hf.co/datasets/squad", KindDataset, "squad"},
		{"https://www.huggingface.co/spaces/a/b", KindSpace, "a/b"},
		{"https://huggingface.co/collections/google/gemma-1234", KindCollection, "google/gemma-1234"},
		{"https://huggingface.co/papers/2401.02412", KindPaper, "2401.02412"},
		{"https://huggingface.co/blog/train-your-own", KindBlog, "train-your-own"},
		{"https://huggingface.co/tasks/image-classification", KindTask, "image-classification"},

		// Sub-pages of a repo name the thing on the page, not the repo.
		{"https://huggingface.co/a/b/discussions/12", KindDiscussion, "model/a/b#12"},
		{"https://huggingface.co/a/b/commit/deadbeef", KindCommit, "model/a/b@deadbeef"},
		{"https://huggingface.co/a/b/tree/main", KindRef, "model/a/b@main"},
		{"https://huggingface.co/a/b/blob/main/README.md", KindFile, "model/a/b@main/README.md"},
		{"https://huggingface.co/datasets/a/b/blob/main/x.csv", KindFile, "dataset/a/b@main/x.csv"},

		// arXiv, in every form the hub links to.
		{"2401.02412", KindPaper, "2401.02412"},
		{"https://arxiv.org/abs/2401.02412", KindPaper, "2401.02412"},
		{"https://arxiv.org/abs/2401.02412v3", KindPaper, "2401.02412"},
		{"https://arxiv.org/pdf/2401.02412v1", KindPaper, "2401.02412"},

		// Whitespace and trailing slashes are what a paste looks like.
		{"  google-bert/bert-base-uncased/  ", KindModel, "google-bert/bert-base-uncased"},
	}
	for _, c := range cases {
		kind, id, err := Classify(c.in)
		if err != nil {
			t.Errorf("Classify(%q): %v", c.in, err)
			continue
		}
		if kind != c.kind || id != c.id {
			t.Errorf("Classify(%q) = %q %q, want %q %q", c.in, kind, id, c.kind, c.id)
		}
	}
}

func TestClassifyRejects(t *testing.T) {
	for _, in := range []string{
		"",
		"   ",
		"docs",                           // site chrome, not a namespace
		"https://huggingface.co/pricing", // same, through a URL
		"https://example.com/a/b",        // not the hub at all
		"https://myspace-demo.hf.space",  // not reversible to owner/name
		"a/b/c/d/e",                      // too many segments to guess
		"https://arxiv.org/",             // no id in it
	} {
		if kind, id, err := Classify(in); err == nil {
			t.Errorf("Classify(%q) = %q %q, want an error", in, kind, id)
		}
	}
}

// TestLocateRoundTrip is the property that matters: every URI a record carries
// leads to a page, and that page classifies back to the same URI. A kind that
// breaks the loop is one whose -o url output goes somewhere wrong.
func TestLocateRoundTrip(t *testing.T) {
	cases := []struct {
		kind string
		id   string
		url  string
	}{
		{KindModel, "google-bert/bert-base-uncased", BaseURL + "/google-bert/bert-base-uncased"},
		{KindDataset, "squad", BaseURL + "/datasets/squad"},
		{KindSpace, "a/b", BaseURL + "/spaces/a/b"},
		{KindKernel, "a/b", BaseURL + "/kernels/a/b"},
		{KindOrg, "google", BaseURL + "/google"},
		{KindCollection, "google/gemma-1234", BaseURL + "/collections/google/gemma-1234"},
		{KindPaper, "2401.02412", BaseURL + "/papers/2401.02412"},
		{KindBlog, "train-your-own", BaseURL + "/blog/train-your-own"},
		{KindTask, "image-classification", BaseURL + "/tasks/image-classification"},
		{KindDiscussion, "model/a/b#12", BaseURL + "/a/b/discussions/12"},
		{KindCommit, "model/a/b@deadbeef", BaseURL + "/a/b/commit/deadbeef"},
		{KindRef, "dataset/a/b@main", BaseURL + "/datasets/a/b/tree/main"},
		{KindFile, "model/a/b@main/README.md", BaseURL + "/a/b/blob/main/README.md"},
	}
	for _, c := range cases {
		got, err := Locate(c.kind, c.id)
		if err != nil {
			t.Errorf("Locate(%q, %q): %v", c.kind, c.id, err)
			continue
		}
		if got != c.url {
			t.Errorf("Locate(%q, %q) = %q, want %q", c.kind, c.id, got, c.url)
			continue
		}
		// An org and a user share a URL shape, so a bare name comes back as a
		// namespace. Everything else is expected to survive the round trip.
		kind, id, err := Classify(got)
		if err != nil {
			t.Errorf("Classify(%q): %v", got, err)
			continue
		}
		if c.kind == KindOrg {
			kind = c.kind
		}
		if kind != c.kind || id != c.id {
			t.Errorf("Classify(Locate(%q, %q)) = %q %q, want the input back", c.kind, c.id, kind, id)
		}
	}
}

func TestLocateRejectsBadIDs(t *testing.T) {
	for _, c := range []struct{ kind, id string }{
		{KindModel, ""},
		{KindDiscussion, "a/b"},      // no number
		{KindCommit, "model/a/b"},    // no revision
		{KindFile, "model/a/b@main"}, // no path
	} {
		if got, err := Locate(c.kind, c.id); err == nil {
			t.Errorf("Locate(%q, %q) = %q, want an error", c.kind, c.id, got)
		}
	}
}

func TestParseTag(t *testing.T) {
	tax := &Taxonomy{byID: map[string]string{
		"transformers":    TypeLibrary,
		"text-generation": TypePipeline,
		"en":              TypeLanguage,
	}}
	cases := []struct {
		raw    string
		ns     string
		rel    string
		value  string
		kind   string
		target string
	}{
		// The namespaced tags, which are where the edges come from.
		{"license:mit", TypeLicense, "", "mit", TypeLicense, ""},
		{"arxiv:2401.02412", TypeArxiv, "", "2401.02412", KindPaper, "hf://paper/2401.02412"},
		{"arxiv:2401.02412v2", TypeArxiv, "", "2401.02412", KindPaper, "hf://paper/2401.02412"},
		{"doi:10.57967/hf/1234", TypeDOI, "", "10.57967/hf/1234", TypeDOI, ""},
		{"dataset:squad", TypeDataset, "", "squad", KindDataset, "hf://dataset/squad"},
		{"region:us", TypeRegion, "", "us", TypeRegion, ""},
		{"language:fr", TypeLanguage, "", "fr", TypeLanguage, ""},

		// base_model carries a relation, and drops to a plain base when it does not.
		{"base_model:finetune:meta-llama/Llama-3-8B", TypeBase, "finetune", "meta-llama/Llama-3-8B", KindModel, "hf://model/meta-llama/Llama-3-8B"},
		{"base_model:quantized:a/b", TypeBase, "quantized", "a/b", KindModel, "hf://model/a/b"},
		{"base_model:a/b", TypeBase, "", "a/b", KindModel, "hf://model/a/b"},

		// An unknown namespace stays a namespace rather than collapsing.
		{"trl:SFT", "trl", "", "SFT", TypeOther, ""},

		// Bare words go through the taxonomy, then fall through to other.
		{"transformers", "", "", "transformers", TypeLibrary, ""},
		{"text-generation", "", "", "text-generation", TypePipeline, ""},
		{"en", "", "", "en", TypeLanguage, ""},
		{"vision-language", "", "", "vision-language", TypeOther, ""},

		// A capability flag is a fact about the repo, not a link to anything.
		{"endpoints_compatible", "", "", "endpoints_compatible", TypeOther, ""},
	}
	for _, c := range cases {
		p := ParseTag(c.raw, tax)
		if p.Namespace != c.ns || p.Relation != c.rel || p.Value != c.value || p.Kind != c.kind || p.TargetURI != c.target {
			t.Errorf("ParseTag(%q) = ns %q rel %q value %q kind %q target %q, want %q %q %q %q %q",
				c.raw, p.Namespace, p.Relation, p.Value, p.Kind, p.TargetURI, c.ns, c.rel, c.value, c.kind, c.target)
		}
		if p.Raw != c.raw {
			t.Errorf("ParseTag(%q) lost the raw tag: %q", c.raw, p.Raw)
		}
	}
}

// TestParseTagWithoutTaxonomy pins the degradation. Without the vocabulary
// loaded a bare word is a free tag, which is less information than a library,
// but it is not an error and it must not panic on the nil.
func TestParseTagWithoutTaxonomy(t *testing.T) {
	p := ParseTag("transformers", nil)
	if p.Kind != TypeOther {
		t.Errorf("kind = %q, want %q without a taxonomy", p.Kind, TypeOther)
	}
	if p := ParseTag("license:apache-2.0", nil); p.Kind != TypeLicense {
		t.Errorf("a namespaced tag needs no taxonomy, got kind %q", p.Kind)
	}
}

func TestGatedDecode(t *testing.T) {
	cases := []struct {
		in   string
		want Gated
	}{
		{`false`, NotGated},
		{`null`, NotGated},
		{`""`, NotGated},
		{`true`, GatedAuto},
		{`"auto"`, GatedAuto},
		{`"manual"`, GatedManual},
	}
	for _, c := range cases {
		var g Gated
		if err := json.Unmarshal([]byte(c.in), &g); err != nil {
			t.Errorf("Gated(%s): %v", c.in, err)
			continue
		}
		if g != c.want {
			t.Errorf("Gated(%s) = %q, want %q", c.in, g, c.want)
		}
		if got := g.IsGated(); got != (c.want != NotGated) {
			t.Errorf("Gated(%s).IsGated() = %v", c.in, got)
		}
	}
}

func TestStringListDecode(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		// The same key, written both ways, sometimes in the same repository.
		{`"mit"`, []string{"mit"}},
		{`["mit"]`, []string{"mit"}},
		{`["mit","apache-2.0"]`, []string{"mit", "apache-2.0"}},

		// Nothing to say, said several ways.
		{`null`, nil},
		{`[]`, []string{}},
		{`""`, nil},

		// A card author who wrote a number, or left a hole in a list.
		{`2`, []string{"2"}},
		{`["mit",null,"apache-2.0"]`, []string{"mit", "apache-2.0"}},
	}
	for _, c := range cases {
		var got StringList
		if err := json.Unmarshal([]byte(c.in), &got); err != nil {
			t.Errorf("StringList(%s): %v", c.in, err)
			continue
		}
		if len(got) != len(c.want) {
			t.Errorf("StringList(%s) = %v, want %v", c.in, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("StringList(%s) = %v, want %v", c.in, got, c.want)
				break
			}
		}
	}
}

func TestTimeDecode(t *testing.T) {
	want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	cases := []struct {
		in   string
		want time.Time
	}{
		{`"2024-01-02T03:04:05Z"`, want},
		{`"2024-01-02T03:04:05.000Z"`, want},
		{`"2024-01-02T03:04:05.123456Z"`, want.Add(123456 * time.Microsecond)},
		{`1704164645`, want},    // seconds
		{`1704164645000`, want}, // milliseconds
		{`"2024-01-02"`, time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},
		{`null`, time.Time{}},
		{`""`, time.Time{}},
		{`"not a date"`, time.Time{}}, // never an error, the record keeps going
	}
	for _, c := range cases {
		var got Time
		if err := json.Unmarshal([]byte(c.in), &got); err != nil {
			t.Errorf("Time(%s): %v", c.in, err)
			continue
		}
		if !got.Equal(c.want) {
			t.Errorf("Time(%s) = %v, want %v", c.in, got.Time, c.want)
		}
	}
}

func TestTimeMarshalsNullForZero(t *testing.T) {
	b, err := json.Marshal(Time{})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "null" {
		t.Errorf("zero Time marshalled to %s, want null", b)
	}
}

func TestURISplitRoundTrip(t *testing.T) {
	for _, c := range []struct{ kind, id string }{
		{KindModel, "a/b"},
		{KindDataset, "squad"},
		{KindDiscussion, "model/a/b#12"},
		{KindFile, "model/a/b@main/path/to/file.txt"},
		{KindTag, "license:mit"},
	} {
		u := URI(c.kind, c.id)
		kind, id, ok := SplitURI(u)
		if !ok || kind != c.kind || id != c.id {
			t.Errorf("SplitURI(%q) = %q %q %v, want %q %q true", u, kind, id, ok, c.kind, c.id)
		}
	}
	for _, s := range []string{"", "a/b", "https://huggingface.co/a/b", "hf://", "hf://model"} {
		if kind, id, ok := SplitURI(s); ok {
			t.Errorf("SplitURI(%q) = %q %q true, want false", s, kind, id)
		}
	}
}
