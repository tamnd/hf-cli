package hf

import (
	"encoding/json"
	"strings"
	"time"
)

// user.go covers the two namespace kinds and the compact user object that
// appears embedded in nearly every other record.

// User is a hub account. The overview endpoint does not return the name,
// because it is the path parameter, so the client fills it in.
type User struct {
	Meta

	Name      string `json:"name"`
	ObjectID  string `json:"_id,omitempty"`
	Fullname  string `json:"fullname,omitempty"`
	AvatarURL string `json:"avatarUrl,omitempty"`
	Details   string `json:"details,omitempty"`
	IsPro     bool   `json:"isPro,omitempty"`
	IsHf      bool   `json:"isHf,omitempty"`
	IsMod     bool   `json:"isMod,omitempty"`
	Type      string `json:"type,omitempty"`

	// The overview repeats the name under a second key and adds the account's
	// age, which is the only place on the hub a person's join date appears.
	Handle              string    `json:"user,omitempty"`
	CreatedAt           time.Time `json:"createdAt,omitzero"`
	PrimaryOrgAvatarURL string    `json:"primaryOrgAvatarUrl,omitempty"`
	// IsFollowing is relative to the token making the request, so it is empty
	// for an anonymous read and true only for the caller's own following list.
	IsFollowing bool `json:"isFollowing,omitempty"`

	NumModels        int `json:"numModels"`
	NumDatasets      int `json:"numDatasets"`
	NumSpaces        int `json:"numSpaces"`
	NumKernels       int `json:"numKernels"`
	NumBuckets       int `json:"numBuckets,omitempty"`
	NumPapers        int `json:"numPapers"`
	NumDiscussions   int `json:"numDiscussions"`
	NumUpvotes       int `json:"numUpvotes"`
	NumLikes         int `json:"numLikes"`
	NumFollowers     int `json:"numFollowers"`
	NumFollowing     int `json:"numFollowing"`
	NumFollowingOrgs int `json:"numFollowingOrgs,omitempty"`

	Orgs []UserRef `json:"orgs,omitempty"`

	// Page-derived. The profile carries the first page of each list inline, so a
	// deep fetch of a namespace answers what three list calls would.
	CommunityScore int          `json:"communityScore,omitempty"`
	Activities     []Activity   `json:"activities,omitempty"`
	BlogPosts      []BlogRef    `json:"blogPosts,omitempty"`
	TotalBlogPosts int          `json:"totalBlogPosts,omitempty"`
	Models         []Model      `json:"models,omitempty"`
	Datasets       []Dataset    `json:"datasets,omitempty"`
	Spaces         []Space      `json:"spaces,omitempty"`
	Collections    []Collection `json:"collections,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (u *User) UnmarshalJSON(b []byte) error {
	type raw User
	return decodeExtra(b, (*raw)(u), &u.Extra)
}

// Org is a hub organisation.
type Org struct {
	Meta

	Name         string `json:"name"`
	ObjectID     string `json:"_id,omitempty"`
	Fullname     string `json:"fullname,omitempty"`
	Details      string `json:"details,omitempty"`
	AvatarURL    string `json:"avatarUrl,omitempty"`
	IsVerified   bool   `json:"isVerified,omitempty"`
	IsEnterprise bool   `json:"isEnterprise,omitempty"`
	Plan         string `json:"plan,omitempty"`
	Type         string `json:"type,omitempty"`

	// IsFollowing is relative to the token making the request.
	IsFollowing bool `json:"isFollowing,omitempty"`

	NumUsers     int `json:"numUsers"`
	NumModels    int `json:"numModels"`
	NumDatasets  int `json:"numDatasets"`
	NumSpaces    int `json:"numSpaces"`
	NumKernels   int `json:"numKernels"`
	NumBuckets   int `json:"numBuckets,omitempty"`
	NumPapers    int `json:"numPapers"`
	NumFollowers int `json:"numFollowers"`

	// Page-derived. The org page is the single biggest win in the tool: one
	// request replaces the overview, the member list, a follower sample, three
	// repo list calls, a collections call, and a papers call.
	Card            string       `json:"card,omitempty"`
	Members         []UserRef    `json:"members,omitempty"`
	SampleFollowers []UserRef    `json:"sampleFollowers,omitempty"`
	Models          []Model      `json:"models,omitempty"`
	Datasets        []Dataset    `json:"datasets,omitempty"`
	Spaces          []Space      `json:"spaces,omitempty"`
	Collections     []Collection `json:"collections,omitempty"`
	Papers          []Paper      `json:"papers,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (o *Org) UnmarshalJSON(b []byte) error {
	type raw Org
	return decodeExtra(b, (*raw)(o), &o.Extra)
}

// UserRef is the compact namespace object embedded everywhere: as a liker, a
// follower, a member, a commit author, a discussion author, a repo owner.
type UserRef struct {
	Name          string `json:"name"`
	ObjectID      string `json:"_id,omitempty"`
	Fullname      string `json:"fullname,omitempty"`
	Type          string `json:"type,omitempty"`
	AvatarURL     string `json:"avatarUrl,omitempty"`
	IsPro         bool   `json:"isPro,omitempty"`
	IsHf          bool   `json:"isHf,omitempty"`
	IsHfAdmin     bool   `json:"isHfAdmin,omitempty"`
	IsMod         bool   `json:"isMod,omitempty"`
	IsEnterprise  bool   `json:"isEnterprise,omitempty"`
	Plan          string `json:"plan,omitempty"`
	FollowerCount int    `json:"followerCount,omitempty"`

	// IsUserFollowing is relative to the token making the request.
	IsUserFollowing bool `json:"isUserFollowing,omitempty"`

	URI string `json:"uri,omitempty"`
	URL string `json:"url,omitempty"`
}

// UnmarshalJSON handles the one inconsistency that would otherwise cost half
// the social graph: the namespace name arrives as "user" on likers, followers,
// members and following, as "name" on authors and owners, and as both on some.
// The avatar has the same problem under two names, and a paper's organization
// is the one payload that uses the short one.
func (u *UserRef) UnmarshalJSON(b []byte) error {
	type raw UserRef
	var r raw
	if err := json.Unmarshal(b, &r); err != nil {
		// A bare string is also a valid reference in a few payloads.
		var s string
		if json.Unmarshal(b, &s) == nil {
			*u = UserRef{Name: s}
			u.normalize()
			return nil
		}
		return err
	}
	*u = UserRef(r)
	if u.Name == "" || u.AvatarURL == "" {
		var alt struct {
			User   string `json:"user"`
			Avatar string `json:"avatar"`
		}
		if json.Unmarshal(b, &alt) == nil {
			if u.Name == "" {
				u.Name = alt.User
			}
			if u.AvatarURL == "" {
				u.AvatarURL = alt.Avatar
			}
		}
	}
	u.normalize()
	return nil
}

// normalize fills the address fields and absolutises the avatar, which arrives
// sometimes as a site-relative path and sometimes as a CDN URL.
func (u *UserRef) normalize() {
	if u.Name == "" {
		return
	}
	kind := KindNamespace
	switch u.Type {
	case "user":
		kind = KindUser
	case "org":
		kind = KindOrg
	}
	u.URI = URI(kind, u.Name)
	u.URL = BaseURL + "/" + u.Name
	if strings.HasPrefix(u.AvatarURL, "/") {
		u.AvatarURL = BaseURL + u.AvatarURL
	}
}

// Kind reports which namespace kind this reference is, or KindNamespace when
// the payload did not say.
func (u *UserRef) Kind() string {
	switch u.Type {
	case "user":
		return KindUser
	case "org":
		return KindOrg
	default:
		return KindNamespace
	}
}

// Activity is one entry of a user's page activity feed. There is no API for
// this, and it is the closest thing the hub has to an event log.
type Activity struct {
	Type      string          `json:"type"`
	CreatedAt time.Time       `json:"createdAt,omitzero"`
	Target    string          `json:"target,omitempty"`
	TargetURI string          `json:"targetUri,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// BlogRef is a blog post as it appears on an author's profile.
type BlogRef struct {
	Slug        string    `json:"slug"`
	Title       string    `json:"title,omitempty"`
	PublishedAt time.Time `json:"publishedAt,omitzero"`
	Upvotes     int       `json:"upvotes,omitempty"`
	URI         string    `json:"uri,omitempty"`
	URL         string    `json:"url,omitempty"`
}

// Whoami is the identity behind the current token.
type Whoami struct {
	Meta

	Name string `json:"name"`
	// Whoami is the one endpoint that names the object id "id" rather than "_id".
	ObjectID  string          `json:"id,omitempty"`
	Fullname  string          `json:"fullname,omitempty"`
	Email     string          `json:"email,omitempty"`
	Type      string          `json:"type,omitempty"`
	IsPro     bool            `json:"isPro,omitempty"`
	CanPay    bool            `json:"canPay,omitempty"`
	AvatarURL string          `json:"avatarUrl,omitempty"`
	Orgs      []UserRef       `json:"orgs,omitempty"`
	Auth      json.RawMessage `json:"auth,omitempty"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (w *Whoami) UnmarshalJSON(b []byte) error {
	type raw Whoami
	return decodeExtra(b, (*raw)(w), &w.Extra)
}
