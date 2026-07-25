package hf

import (
	"strconv"
	"strings"
	"time"
)

// graph.go turns records into triples. The hub is already a knowledge graph: a
// model names the dataset it trained on, the paper it implements, and the model
// it was fine-tuned from. This file reads those declarations off a record and
// emits them as typed edges, and it is deliberately pure so it can be tested
// against fixtures with no network at all.

// Node is one entity in the graph. Props carries the small set of attributes
// worth having inline, and the full record is one hf call away by URI.
type Node struct {
	URI   string         `json:"uri" kit:"id" table:"uri"`
	Kind  string         `json:"kind" table:"kind"`
	Label string         `json:"label,omitempty" table:"label,truncate"`
	URL   string         `json:"url,omitempty" table:"-,url"`
	Props map[string]any `json:"props,omitempty" table:"-"`
}

// Edge is one directed, typed relation. Source records how hf knows it, which
// matters because a tag-derived edge and an API-declared edge deserve different
// trust.
type Edge struct {
	Subject    string         `json:"subject" table:"subject"`
	Predicate  string         `json:"predicate" table:"predicate"`
	Object     string         `json:"object" table:"object"`
	ObjectKind string         `json:"objectKind,omitempty" table:"objectKind"`
	Literal    bool           `json:"literal,omitempty" table:"-"`
	Source     string         `json:"source" table:"source"`
	Props      map[string]any `json:"props,omitempty" table:"-"`
}

// The edge sources, in descending order of who asserted the claim. An edge from
// the baseModels expand is asserted by the hub, the same edge from a base_model
// tag is asserted by the repo author, and one inferred from a count is asserted
// by this tool.
const (
	SrcAPI     = "api"
	SrcTag     = "tag"
	SrcCard    = "card"
	SrcPage    = "page"
	SrcDerived = "derived"
)

// The predicate vocabulary. This is the complete set: an edge hf can emit has
// its predicate here, and adding a relation means adding a constant first.
const (
	// Ownership and membership.
	PredOwnedBy       = "hf:ownedBy"
	PredMemberOf      = "hf:memberOf"
	PredContains      = "hf:contains"
	PredHasFile       = "hf:hasFile"
	PredHasRef        = "hf:hasRef"
	PredHasCommit     = "hf:hasCommit"
	PredHasDiscussion = "hf:hasDiscussion"
	PredHasSplit      = "hf:hasSplit"

	// Derivation.
	PredDerivesFrom    = "hf:derivesFrom"
	PredFinetuneOf     = "hf:finetuneOf"
	PredAdapterOf      = "hf:adapterOf"
	PredQuantizationOf = "hf:quantizationOf"
	PredMergeOf        = "hf:mergeOf"
	PredTrainedOn      = "hf:trainedOn"
	PredEvaluatedOn    = "hf:evaluatedOn"
	PredCites          = "hf:cites"
	PredAliasOf        = "hf:aliasOf"

	// Use and capability.
	PredUses       = "hf:uses"
	PredUsedBy     = "hf:usedBy"
	PredServedBy   = "hf:servedBy"
	PredHasTask    = "hf:hasTask"
	PredHasLibrary = "hf:hasLibrary"
	PredHasTag     = "hf:hasTag"
	PredLicense    = "hf:license"
	PredLanguage   = "hf:language"
	PredRegion     = "hf:region"

	// Social.
	PredLikes          = "hf:likes"
	PredFollows        = "hf:follows"
	PredUpvotes        = "hf:upvotes"
	PredAuthorOf       = "hf:authorOf"
	PredClaimedBy      = "hf:claimedBy"
	PredMentions       = "hf:mentions"
	PredRepliesTo      = "hf:repliesTo"
	PredSubmittedDaily = "hf:submittedDaily"

	// Evaluation, reified through a blank node because a result is a four way
	// relation between a model, a task, a dataset, and a number.
	PredHasResult = "hf:hasResult"
	PredOnTask    = "hf:onTask"
	PredOnDataset = "hf:onDataset"
	PredMetric    = "hf:metric"
	PredValue     = "hf:value"
	PredVerified  = "hf:verified"

	// Literals.
	PredDownloads     = "hf:downloads"
	PredDownloadsAll  = "hf:downloadsAllTime"
	PredLikeCount     = "hf:likeCount"
	PredTrendingScore = "hf:trendingScore"
	PredUsedStorage   = "hf:usedStorage"
	PredNumParameters = "hf:numParameters"
	PredNumRows       = "hf:numRows"
	PredCreatedAt     = "hf:createdAt"
	PredLastModified  = "hf:lastModified"
	PredPublishedAt   = "hf:publishedAt"
	PredPrivate       = "hf:private"
	PredGated         = "hf:gated"
	PredSDK           = "hf:sdk"
	PredRuntimeStage  = "hf:runtimeStage"
	PredTitle         = "hf:title"
)

// StructuralPredicates is the crawler's default follow set. Social predicates
// fan out hard, so they stay opt-in.
var StructuralPredicates = []string{PredOwnedBy, PredContains, PredHasFile, PredHasSplit}

// baseRelationPredicate maps a base_model relation to its specific predicate.
// The general derivesFrom is always emitted too, so a consumer that only wants
// lineage queries one predicate and one that cares how queries four.
var baseRelationPredicate = map[string]string{
	RelFinetune:  PredFinetuneOf,
	RelAdapter:   PredAdapterOf,
	RelQuantized: PredQuantizationOf,
	RelMerge:     PredMergeOf,
}

// builder accumulates a node and its edges while an extractor walks a record.
type builder struct {
	node  Node
	edges []Edge
	tax   *Taxonomy
}

func (b *builder) to(pred, objKind, objID, source string) {
	if objID == "" {
		return
	}
	b.edges = append(b.edges, Edge{
		Subject:    b.node.URI,
		Predicate:  pred,
		Object:     URI(objKind, objID),
		ObjectKind: objKind,
		Source:     source,
	})
}

// toURI is the same as to for an object whose URI is already built.
func (b *builder) toURI(pred, objKind, uri, source string) {
	if uri == "" {
		return
	}
	b.edges = append(b.edges, Edge{
		Subject:    b.node.URI,
		Predicate:  pred,
		Object:     uri,
		ObjectKind: objKind,
		Source:     source,
	})
}

// lit emits a literal edge. An empty value is skipped, because "this repo has
// no license" is better said by the absence of a triple than by an empty one.
func (b *builder) lit(pred, value, source string) {
	if value == "" {
		return
	}
	b.edges = append(b.edges, Edge{
		Subject:   b.node.URI,
		Predicate: pred,
		Object:    value,
		Literal:   true,
		Source:    source,
	})
}

func (b *builder) num(pred string, n int64) {
	if n == 0 {
		return
	}
	b.lit(pred, strconv.FormatInt(n, 10), SrcDerived)
}

func (b *builder) at(pred string, t time.Time) {
	if t.IsZero() {
		return
	}
	b.lit(pred, t.UTC().Format(time.RFC3339), SrcDerived)
}

// from emits an edge whose subject is not this node. A follower edge points at
// this node rather than away from it, and inverting it would be a lie.
func (b *builder) from(subject, pred, objURI, objKind, source string) {
	if subject == "" || objURI == "" {
		return
	}
	b.edges = append(b.edges, Edge{
		Subject:    subject,
		Predicate:  pred,
		Object:     objURI,
		ObjectKind: objKind,
		Source:     source,
	})
}

// Extract turns one record into its node and its edges. It is pure: no network,
// no ordering dependency, and no state beyond the tag taxonomy passed in. That
// purity is what makes the graph testable against fixtures.
func Extract(rec any, tax *Taxonomy) (Node, []Edge) {
	b := &builder{tax: tax}
	switch r := rec.(type) {
	case *Model:
		b.model(r)
	case *Dataset:
		b.dataset(r)
	case *Space:
		b.space(r)
	case *Kernel:
		b.kernel(r)
	case *User:
		b.user(r)
	case *Org:
		b.org(r)
	case *Collection:
		b.collection(r)
	case *Paper:
		b.paper(r)
	case *Post:
		b.post(r)
	case *BlogPost:
		b.blog(r)
	case *Discussion:
		b.discussion(r)
	case *Like:
		b.like(r)
	case *Tag:
		b.tag(r)
	case *Task:
		b.task(r)
	case *Split:
		b.split(r)
	case *Size:
		b.size(r)
	case *Commit:
		b.commit(r)
	case *TreeEntry:
		b.file(r)
	default:
		return Node{}, nil
	}
	return b.node, b.edges
}

// repoBase covers everything the four repo kinds share: the node itself, the
// owner edge, the tag edges, the card edges, and the counts.
func (b *builder) repoBase(r *Repo) {
	b.node = Node{URI: r.URI, Kind: r.Kind, Label: r.ID, URL: r.URL}

	// Namespace prefix. The kind of the owner comes from authorData when the
	// record has it, and stays "namespace" otherwise, which is honest rather
	// than wrong.
	if r.Author != "" {
		kind := KindNamespace
		if r.AuthorData != nil {
			kind = r.AuthorData.Kind()
		}
		b.to(PredOwnedBy, kind, r.Author, SrcAPI)
	}
	if r.AliasOf != "" && r.AliasOf != r.ID {
		b.to(PredAliasOf, r.Kind, r.AliasOf, SrcAPI)
	}

	b.tags(r.Tags)
	b.card(r)

	b.num(PredDownloads, int64(r.Downloads))
	b.num(PredDownloadsAll, int64(r.DownloadsAll))
	b.num(PredLikeCount, int64(r.Likes))
	b.num(PredUsedStorage, r.UsedStorage)
	if r.TrendingScore != 0 {
		b.lit(PredTrendingScore, strconv.FormatFloat(r.TrendingScore, 'f', -1, 64), SrcDerived)
	}
	b.at(PredCreatedAt, r.CreatedAt)
	b.at(PredLastModified, r.LastModified)
	if r.Private {
		b.lit(PredPrivate, "true", SrcDerived)
	}
	if r.Gated.IsGated() {
		b.lit(PredGated, string(r.Gated), SrcDerived)
	}
	if r.Region != "" {
		b.to(PredRegion, KindTag, TagID(TypeRegion, r.Region), SrcTag)
	}
	if r.DiscussionsStats != nil {
		b.num("hf:discussionCount", int64(r.DiscussionsStats.Total))
	}
}

// tags is where most of the graph's volume comes from. A typical model carries
// 15 to 40 tags and perhaps four of them are prose. Every tag produces a hasTag
// edge unconditionally so the raw declaration survives, and a tag whose
// namespace maps to a relation additionally produces the typed edge.
func (b *builder) tags(raw []string) {
	for _, p := range ParseTags(raw, b.tax) {
		b.toURI(PredHasTag, KindTag, p.TagURI(), SrcAPI)
		switch p.Kind {
		case KindPaper:
			b.toURI(PredCites, KindPaper, p.TargetURI, SrcTag)
		case KindDataset:
			b.toURI(PredTrainedOn, KindDataset, p.TargetURI, SrcTag)
		case KindModel:
			b.toURI(PredDerivesFrom, KindModel, p.TargetURI, SrcTag)
			if pred, ok := baseRelationPredicate[p.Relation]; ok {
				b.toURI(pred, KindModel, p.TargetURI, SrcTag)
			}
		case TypeLicense:
			b.to(PredLicense, KindTag, TagID(TypeLicense, p.Value), SrcTag)
		case TypeLanguage:
			b.to(PredLanguage, KindTag, TagID(TypeLanguage, p.Value), SrcTag)
		case TypeLibrary:
			b.to(PredHasLibrary, KindTag, TagID(TypeLibrary, p.Value), SrcTag)
		case TypePipeline:
			b.to(PredHasTask, KindTask, p.Value, SrcTag)
		case TypeRegion:
			b.to(PredRegion, KindTag, TagID(TypeRegion, p.Value), SrcTag)
		}
	}
}

// card reads the front matter. It often says something the tag array does not,
// and occasionally contradicts it. A contradiction is not resolved here: both
// edges are emitted with different sources, because silently picking a winner
// would hide a real disagreement in the data.
func (b *builder) card(r *Repo) {
	c := r.CardData
	if c == nil {
		return
	}
	for _, ref := range c.BaseModelRefs() {
		b.to(PredDerivesFrom, KindModel, ref.ID, SrcCard)
		if pred, ok := baseRelationPredicate[ref.Relation]; ok {
			b.to(pred, KindModel, ref.ID, SrcCard)
		}
	}
	for _, ds := range c.Datasets {
		b.to(PredTrainedOn, KindDataset, ds, SrcCard)
	}
	if id := c.LicenseID(); id != "" {
		b.to(PredLicense, KindTag, TagID(TypeLicense, strings.ToLower(id)), SrcCard)
	}
	for _, lang := range c.Language {
		b.to(PredLanguage, KindTag, TagID(TypeLanguage, strings.ToLower(lang)), SrcCard)
	}
	if c.PipelineTag != "" {
		b.to(PredHasTask, KindTask, c.PipelineTag, SrcCard)
	}
	if c.LibraryName != "" {
		b.to(PredHasLibrary, KindTag, TagID(TypeLibrary, c.LibraryName), SrcCard)
	}
	for _, task := range c.TaskCategories {
		b.to(PredHasTask, KindTask, task, SrcCard)
	}
	for _, m := range c.Models {
		b.to(PredUses, KindModel, m, SrcCard)
	}
}

func (b *builder) model(m *Model) {
	b.repoBase(&m.Repo)

	if m.PipelineTag != "" {
		b.to(PredHasTask, KindTask, m.PipelineTag, SrcAPI)
	}
	if m.Library != "" {
		b.to(PredHasLibrary, KindTag, TagID(TypeLibrary, m.Library), SrcAPI)
	}
	for _, ref := range m.BaseModels {
		b.to(PredDerivesFrom, KindModel, ref.ID, SrcAPI)
		if pred, ok := baseRelationPredicate[ref.Relation]; ok {
			b.to(pred, KindModel, ref.ID, SrcAPI)
		}
	}
	for _, id := range m.Spaces {
		b.to(PredUsedBy, KindSpace, id, SrcAPI)
	}
	for _, s := range m.LinkedSpaces {
		b.to(PredUsedBy, KindSpace, s.ID, SrcPage)
	}
	for _, p := range m.InferenceProviders {
		b.to(PredServedBy, KindProvider, p.Provider, SrcAPI)
	}
	if m.Safetensors != nil {
		b.num(PredNumParameters, m.Safetensors.Total)
	} else {
		b.num(PredNumParameters, m.NumParameters)
	}

	// Evaluation results are reified: a result is a relation between a model, a
	// task, a dataset, and a number, which is exactly what a binary edge cannot
	// hold. This is the only place the graph uses blank nodes.
	for i, res := range m.EvalResults {
		blank := "_:res-" + strings.ReplaceAll(m.ID, "/", "-") + "-" + strconv.Itoa(i)
		b.edges = append(b.edges, Edge{
			Subject: b.node.URI, Predicate: PredHasResult, Object: blank, Source: SrcCard,
		})
		if res.TaskType != "" {
			b.from(blank, PredOnTask, URI(KindTask, res.TaskType), KindTask, SrcCard)
		}
		if res.DatasetType != "" {
			b.from(blank, PredOnDataset, URI(KindDataset, res.DatasetType), KindDataset, SrcCard)
		}
		b.edges = append(b.edges,
			Edge{Subject: blank, Predicate: PredMetric, Object: res.MetricType, Literal: true, Source: SrcCard},
			Edge{Subject: blank, Predicate: PredValue, Object: formatFloat(res.Value), Literal: true, Source: SrcCard},
		)
		if res.Verified {
			b.edges = append(b.edges, Edge{Subject: blank, Predicate: PredVerified, Object: "true", Literal: true, Source: SrcCard})
		}
	}
}

func (b *builder) dataset(d *Dataset) {
	b.repoBase(&d.Repo)
	for _, t := range d.TaskCategories {
		b.to(PredHasTask, KindTask, t, SrcAPI)
	}
	for _, lang := range d.Languages {
		b.to(PredLanguage, KindTag, TagID(TypeLanguage, strings.ToLower(lang)), SrcAPI)
	}
	for _, s := range d.LinkedSpaces {
		b.to(PredUsedBy, KindSpace, s.ID, SrcPage)
	}
	if d.PrettyName != "" {
		b.node.Label = d.PrettyName
	}
}

func (b *builder) space(s *Space) {
	b.repoBase(&s.Repo)
	for _, id := range s.Models {
		b.to(PredUses, KindModel, id, SrcAPI)
	}
	for _, id := range s.Datasets {
		b.to(PredUses, KindDataset, id, SrcAPI)
	}
	b.lit(PredSDK, s.SDK, SrcAPI)
	if s.Runtime != nil {
		b.lit(PredRuntimeStage, s.Runtime.Stage, SrcAPI)
	}
}

func (b *builder) kernel(k *Kernel) {
	b.repoBase(&k.Repo)
}

func (b *builder) user(u *User) {
	label := u.Fullname
	if label == "" {
		label = u.Name
	}
	b.node = Node{URI: u.URI, Kind: KindUser, Label: label, URL: u.URL}
	for _, org := range u.Orgs {
		b.to(PredMemberOf, KindOrg, org.Name, SrcAPI)
	}
	for _, m := range u.Models {
		b.from(m.URI, PredOwnedBy, b.node.URI, KindUser, SrcPage)
	}
	for _, d := range u.Datasets {
		b.from(d.URI, PredOwnedBy, b.node.URI, KindUser, SrcPage)
	}
	for _, s := range u.Spaces {
		b.from(s.URI, PredOwnedBy, b.node.URI, KindUser, SrcPage)
	}
	for _, c := range u.Collections {
		b.from(c.URI, PredOwnedBy, b.node.URI, KindUser, SrcPage)
	}
	for _, bp := range u.BlogPosts {
		b.to(PredAuthorOf, KindBlog, bp.Slug, SrcPage)
	}
	b.activities(u.Activities)
	b.num("hf:followerCount", int64(u.NumFollowers))
}

// activities reads the profile feed, which is the closest thing the hub has to
// an event log and exists nowhere in the API.
func (b *builder) activities(acts []Activity) {
	for _, a := range acts {
		target := a.TargetURI
		if target == "" && a.Target != "" {
			if kind, id, err := Classify(a.Target); err == nil {
				target = URI(kind, id)
			}
		}
		if target == "" {
			continue
		}
		pred := PredAuthorOf
		switch {
		case strings.Contains(a.Type, "like"):
			pred = PredLikes
		case strings.Contains(a.Type, "upvote"):
			pred = PredUpvotes
		}
		e := Edge{Subject: b.node.URI, Predicate: pred, Object: target, Source: SrcPage}
		if !a.CreatedAt.IsZero() {
			e.Props = map[string]any{"createdAt": a.CreatedAt.UTC().Format(time.RFC3339)}
		}
		b.edges = append(b.edges, e)
	}
}

func (b *builder) org(o *Org) {
	label := o.Fullname
	if label == "" {
		label = o.Name
	}
	b.node = Node{URI: o.URI, Kind: KindOrg, Label: label, URL: o.URL}
	for _, m := range o.Members {
		b.from(URI(m.Kind(), m.Name), PredMemberOf, b.node.URI, KindOrg, SrcAPI)
	}
	for _, m := range o.Models {
		b.from(m.URI, PredOwnedBy, b.node.URI, KindOrg, SrcPage)
	}
	for _, d := range o.Datasets {
		b.from(d.URI, PredOwnedBy, b.node.URI, KindOrg, SrcPage)
	}
	for _, s := range o.Spaces {
		b.from(s.URI, PredOwnedBy, b.node.URI, KindOrg, SrcPage)
	}
	for _, c := range o.Collections {
		b.from(c.URI, PredOwnedBy, b.node.URI, KindOrg, SrcPage)
	}
	for _, p := range o.Papers {
		b.to(PredContains, KindPaper, p.ID, SrcPage)
	}
	b.num("hf:followerCount", int64(o.NumFollowers))
	b.num("hf:memberCount", int64(o.NumUsers))
}

func (b *builder) collection(c *Collection) {
	b.node = Node{URI: c.URI, Kind: KindCollection, Label: c.Title, URL: c.URL}
	if c.Owner != nil {
		b.to(PredOwnedBy, c.Owner.Kind(), c.Owner.Name, SrcAPI)
	} else if c.Namespace != "" {
		b.to(PredOwnedBy, KindNamespace, c.Namespace, SrcAPI)
	}
	// A collection is the one place on the hub where a human writes down why two
	// things belong together, so its membership edges carry the note.
	for _, item := range c.Items {
		e := Edge{
			Subject:    b.node.URI,
			Predicate:  PredContains,
			Object:     URI(item.Kind(), item.ID),
			ObjectKind: item.Kind(),
			Source:     SrcAPI,
		}
		if note := item.NoteText(); note != "" {
			e.Props = map[string]any{"note": note, "position": item.Position}
		}
		b.edges = append(b.edges, e)
	}
	for _, u := range c.Upvoters {
		b.from(u.URI, PredUpvotes, b.node.URI, KindCollection, SrcPage)
	}
	b.num(PredLikeCount, int64(c.Upvotes))
	b.at(PredLastModified, c.LastUpdated)
}

func (b *builder) paper(p *Paper) {
	b.node = Node{URI: p.URI, Kind: KindPaper, Label: p.Title, URL: p.URL}
	// A claimed and verified author is one of the few edges on the site a human
	// explicitly confirmed. An unverified name match is not an identity and does
	// not become an edge.
	for _, a := range p.Authors {
		if a.Verified() {
			b.to(PredClaimedBy, a.User.Kind(), a.User.Name, SrcAPI)
		}
	}
	if p.SubmittedOnDailyBy != nil {
		b.from(p.SubmittedOnDailyBy.URI, PredSubmittedDaily, b.node.URI, KindPaper, SrcAPI)
	}
	for _, u := range p.Upvoters {
		b.from(u.URI, PredUpvotes, b.node.URI, KindPaper, SrcPage)
	}
	for _, id := range p.Models {
		b.from(URI(KindModel, id), PredCites, b.node.URI, KindPaper, SrcAPI)
	}
	for _, id := range p.Datasets {
		b.from(URI(KindDataset, id), PredCites, b.node.URI, KindPaper, SrcAPI)
	}
	for _, id := range p.Spaces {
		b.from(URI(KindSpace, id), PredCites, b.node.URI, KindPaper, SrcAPI)
	}
	b.num(PredLikeCount, int64(p.Upvotes))
	b.at(PredPublishedAt, p.PublishedAt)
	b.comments(p.Comments)
}

func (b *builder) post(p *Post) {
	label := p.Body
	if len(label) > 80 {
		label = label[:80]
	}
	b.node = Node{URI: p.URI, Kind: KindPost, Label: label, URL: p.URL}
	if p.Author != nil {
		b.from(p.Author.URI, PredAuthorOf, b.node.URI, KindPost, SrcAPI)
	}
	// Mentions in a post are structural rather than parsed out of prose: the
	// body arrives as a token array and the author typed the reference on
	// purpose, which makes it worth more than a heuristic link extraction.
	for _, tok := range p.Content {
		if tok.Resource != nil && tok.Resource.URI != "" {
			b.toURI(PredMentions, tok.Resource.Type, tok.Resource.URI, SrcAPI)
		}
		if tok.Type == "mention" && tok.User != "" {
			b.to(PredMentions, KindNamespace, tok.User, SrcAPI)
		}
	}
	b.at(PredPublishedAt, p.PublishedAt)
	b.comments(p.Comments)
}

func (b *builder) blog(p *BlogPost) {
	b.node = Node{URI: p.URI, Kind: KindBlog, Label: p.Title, URL: p.URL}
	for _, a := range p.Authors {
		b.from(a.URI, PredAuthorOf, b.node.URI, KindBlog, SrcPage)
	}
	for _, u := range p.Upvoters {
		b.from(u.URI, PredUpvotes, b.node.URI, KindBlog, SrcPage)
	}
	b.tags(p.Tags)
	b.num(PredLikeCount, int64(p.Upvotes))
	b.at(PredPublishedAt, p.PublishedAt)
	b.comments(p.Comments)
}

func (b *builder) discussion(d *Discussion) {
	b.node = Node{URI: d.URI, Kind: KindDiscussion, Label: d.Title, URL: d.URL}
	if d.Repo != nil {
		b.from(d.Repo.URI, PredHasDiscussion, b.node.URI, KindDiscussion, SrcAPI)
	}
	if d.Author != nil {
		b.from(d.Author.URI, PredAuthorOf, b.node.URI, KindDiscussion, SrcAPI)
	}
	for _, e := range d.Events {
		if e.Author != nil && e.Type == "comment" {
			b.from(e.Author.URI, PredRepliesTo, b.node.URI, KindDiscussion, SrcAPI)
		}
	}
	b.at(PredCreatedAt, d.CreatedAt)
}

// comments emits one repliesTo edge per commenter. The comment itself is not a
// node: it has no stable address on the hub, so an edge from its author to the
// thing being commented on is the whole of what can honestly be said.
func (b *builder) comments(cs []Comment) {
	for _, c := range cs {
		if c.Author != nil && c.Author.URI != "" {
			b.from(c.Author.URI, PredRepliesTo, b.node.URI, b.node.Kind, SrcPage)
		}
		for _, m := range c.Mentions {
			if m.URI != "" {
				b.toURI(PredMentions, m.Type, m.URI, SrcPage)
			}
		}
	}
}

func (b *builder) like(l *Like) {
	b.node = Node{URI: URI(KindUser, l.User), Kind: KindUser, Label: l.User, URL: BaseURL + "/" + l.User}
	if l.Repo == nil {
		return
	}
	e := Edge{
		Subject:    b.node.URI,
		Predicate:  PredLikes,
		Object:     l.Repo.URI,
		ObjectKind: l.Repo.Type,
		Source:     SrcAPI,
	}
	if !l.CreatedAt.IsZero() {
		e.Props = map[string]any{"createdAt": l.CreatedAt.UTC().Format(time.RFC3339)}
	}
	b.edges = append(b.edges, e)
}

func (b *builder) tag(t *Tag) {
	label := t.Label
	if label == "" {
		label = t.ID
	}
	b.node = Node{URI: t.URI, Kind: KindTag, Label: label, URL: t.URL}
	// A dataset or paper tag resolves to a real entity, and both the tag node
	// and the edge to that entity are kept: collapsing them would lose the fact
	// that the repo declared a tag rather than a link.
	p := ParseTag(t.ID, b.tax)
	b.toURI("hf:resolvesTo", p.Kind, p.TargetURI, SrcTag)
	b.num("hf:repoCount", int64(t.Count))
}

func (b *builder) task(t *Task) {
	b.node = Node{URI: t.URI, Kind: KindTask, Label: t.Label, URL: t.URL}
	// Curated members are an editorial signal, which is a different thing from a
	// download count and worth its own edges.
	for _, m := range t.Models {
		b.to(PredContains, KindModel, m.ID, SrcAPI)
	}
	for _, d := range t.Datasets {
		b.to(PredContains, KindDataset, d.ID, SrcAPI)
	}
	for _, s := range t.Spaces {
		b.to(PredContains, KindSpace, s.ID, SrcAPI)
	}
	for _, l := range t.Libraries {
		b.to(PredHasLibrary, KindTag, TagID(TypeLibrary, l), SrcAPI)
	}
}

func (b *builder) split(s *Split) {
	b.node = Node{URI: s.URI, Kind: KindSplit, Label: s.Split, URL: s.URL}
	b.from(URI(KindDataset, s.Dataset), PredHasSplit, b.node.URI, KindSplit, SrcAPI)
}

// size is the same node as a split, reached from the size endpoint, which is
// the only place the row count lives. The dataset level and the config level
// are counts about a dataset rather than about a split, so only the split level
// becomes a split node.
func (b *builder) size(s *Size) {
	switch s.Level {
	case "split":
		b.node = Node{URI: s.URI, Kind: KindSplit, Label: s.Split, URL: s.URL}
		b.from(URI(KindDataset, s.Dataset), PredHasSplit, b.node.URI, KindSplit, SrcAPI)
	default:
		b.node = Node{URI: URI(KindDataset, s.Dataset), Kind: KindDataset, Label: s.Dataset}
	}
	b.num(PredNumRows, s.NumRows)
	b.num("hf:numBytes", s.NumBytesOriginalFiles)
}

func (b *builder) commit(c *Commit) {
	b.node = Node{URI: c.URI, Kind: KindCommit, Label: c.Title, URL: c.URL}
	for _, a := range c.Authors {
		if a.User != "" {
			b.from(URI(KindUser, a.User), PredAuthorOf, b.node.URI, KindCommit, SrcAPI)
		}
	}
	b.at(PredCreatedAt, c.Date)
}

func (b *builder) file(t *TreeEntry) {
	b.node = Node{URI: t.URI, Kind: KindFile, Label: t.Path, URL: t.URL}
	b.num("hf:size", t.Size)
	b.lit("hf:oid", t.OID, SrcAPI)
}

func formatFloat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// Graph is a materialised set of nodes and edges. The streaming commands never
// build one, but the graph command for a single entity and the tests both want
// the whole thing in hand.
type Graph struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

// Add folds a record into the graph, skipping a node already present so a
// repeat visit does not duplicate it.
func (g *Graph) Add(rec any, tax *Taxonomy) {
	node, edges := Extract(rec, tax)
	if node.URI == "" {
		return
	}
	seen := false
	for _, n := range g.Nodes {
		if n.URI == node.URI {
			seen = true
			break
		}
	}
	if !seen {
		g.Nodes = append(g.Nodes, node)
	}
	g.Edges = append(g.Edges, edges...)
}

// Targets returns the object URIs reachable under an allowed predicate set,
// which is what the crawler walks. Literal edges are never targets.
func (g *Graph) Targets(allow map[string]bool) []string {
	var out []string
	seen := map[string]bool{}
	for _, e := range g.Edges {
		if e.Literal || strings.HasPrefix(e.Object, "_:") {
			continue
		}
		if len(allow) > 0 && !allow[strings.TrimPrefix(e.Predicate, "hf:")] && !allow[e.Predicate] {
			continue
		}
		if !seen[e.Object] {
			seen[e.Object] = true
			out = append(out, e.Object)
		}
	}
	return out
}
