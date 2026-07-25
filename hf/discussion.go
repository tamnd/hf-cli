package hf

import (
	"encoding/json"
	"strconv"
	"time"
)

// Discussion is one thread on a repo. Pull requests are discussions with a
// branch attached, which is why one type covers both and IsPullRequest is the
// only thing that separates them.
type Discussion struct {
	Meta

	Num           int       `json:"num"`
	ObjectID      string    `json:"objectId,omitempty"`
	Title         string    `json:"title"`
	Status        string    `json:"status"`
	IsPullRequest bool      `json:"isPullRequest"`
	CreatedAt     time.Time `json:"createdAt,omitzero"`
	NumComments   int       `json:"numComments"`
	Pinned        bool      `json:"pinned,omitempty"`
	Locked        bool      `json:"locked,omitempty"`

	// IsReport marks a thread opened through the report button rather than the
	// new-discussion button, which is the one flag that separates moderation
	// traffic from ordinary community traffic.
	IsReport bool `json:"isReport,omitempty"`

	Author    *UserRef `json:"author,omitempty"`
	Repo      *RepoRef `json:"repo,omitempty"`
	RepoOwner *UserRef `json:"repoOwner,omitempty"`

	TopReactions     []Reaction `json:"topReactions,omitempty"`
	NumReactionUsers int        `json:"numReactionUsers,omitempty"`

	Events []DiscussionEvent `json:"events,omitempty"`

	// Pull request fields.
	ChangesCount int    `json:"changesCount,omitempty"`
	TargetBranch string `json:"targetBranch,omitempty"`
	Conflicting  bool   `json:"conflicting,omitempty"`
	Diff         string `json:"diff,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra. The
// detail route and the list route disagree about two names: the object id is
// _id on one and objectId on the other, and the repo owner is org on one and
// repoOwner on the other. Both mean the same thing, so both land in one field.
// The dropped key is the database collection the record lives in, which is the
// same string on every discussion and says nothing about this one.
func (d *Discussion) UnmarshalJSON(b []byte) error {
	type raw Discussion
	if err := decodeExtra(b, (*raw)(d), &d.Extra, "_id", "org", "collection"); err != nil {
		return err
	}
	var alt struct {
		ObjectID string   `json:"_id"`
		Org      *UserRef `json:"org"`
	}
	if json.Unmarshal(b, &alt) == nil {
		if d.ObjectID == "" {
			d.ObjectID = alt.ObjectID
		}
		if d.RepoOwner == nil {
			d.RepoOwner = alt.Org
		}
	}
	return nil
}

// normalize needs the repo passed in, because the listing payload does not name
// the repo it came from. A discussion without its repo has no address.
func (d *Discussion) normalize(repoKind, repo, sourceURL string) {
	if d.Author != nil {
		d.Author.normalize()
	}
	if d.RepoOwner != nil {
		d.RepoOwner.normalize()
	}
	if d.Repo == nil && repo != "" {
		d.Repo = &RepoRef{Name: repo, Type: repoKind}
	}
	if d.Repo != nil {
		if d.Repo.Type == "" {
			d.Repo.Type = repoKind
		}
		d.Repo.normalize()
		repoKind, repo = d.Repo.Type, d.Repo.Name
	}
	for i := range d.Events {
		d.Events[i].normalize()
	}
	if repo == "" {
		return
	}
	d.setMeta(KindDiscussion, DiscussionID(repoKind, repo, d.Num), sourceURL)
}

// DiscussionEvent is one entry of the thread. Upstream sends a heterogeneous
// list, so the shared fields are typed and the payload stays raw, with the
// common comment case also decoded into Comment for convenience.
type DiscussionEvent struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"createdAt,omitzero"`
	Author    *UserRef        `json:"author,omitempty"`
	Comment   *Comment        `json:"comment,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// UnmarshalJSON pulls the comment body out of the flat event object. Upstream
// puts the comment fields on the event itself rather than under a comment key,
// so the convenience field has to be assembled here.
func (e *DiscussionEvent) UnmarshalJSON(b []byte) error {
	type raw DiscussionEvent
	var r raw
	if err := jsonUnmarshal(b, &r); err != nil {
		return err
	}
	*e = DiscussionEvent(r)
	if e.Type == "comment" && e.Comment == nil {
		var flat struct {
			Data struct {
				Latest struct {
					Raw       string    `json:"raw"`
					HTML      string    `json:"html"`
					UpdatedAt time.Time `json:"updatedAt"`
				} `json:"latest"`
				Hidden    bool       `json:"hidden"`
				Edited    bool       `json:"edited"`
				NumEdits  int        `json:"numEdits"`
				Reactions []Reaction `json:"reactions"`
			} `json:"data"`
		}
		if json.Unmarshal(b, &flat) == nil && flat.Data.Latest.Raw != "" {
			e.Comment = &Comment{
				ID:        e.ID,
				Author:    e.Author,
				CreatedAt: e.CreatedAt,
				EditedAt:  flat.Data.Latest.UpdatedAt,
				Raw:       flat.Data.Latest.Raw,
				HTML:      flat.Data.Latest.HTML,
				Hidden:    flat.Data.Hidden,
				Edited:    flat.Data.Edited,
				Reactions: flat.Data.Reactions,
			}
		}
	}
	return nil
}

func (e *DiscussionEvent) normalize() {
	if e.Author != nil {
		e.Author.normalize()
	}
	if e.Comment != nil && e.Comment.Author != nil {
		e.Comment.Author.normalize()
	}
}

// PullRequestBranch is the ref a pull request lives on. The hub names it
// refs/pr/<num>, which is fetchable like any other revision.
func PullRequestBranch(num int) string {
	return "refs/pr/" + strconv.Itoa(num)
}
