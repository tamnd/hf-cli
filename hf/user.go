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

	Name      string `json:"name" table:"name"`
	ObjectID  string `json:"_id,omitempty" table:"-"`
	Fullname  string `json:"fullname,omitempty" table:"fullname"`
	AvatarURL string `json:"avatarUrl,omitempty" table:"-"`
	Details   string `json:"details,omitempty" table:"-"`
	IsPro     bool   `json:"isPro,omitempty" table:"-"`
	IsHf      bool   `json:"isHf,omitempty" table:"-"`
	IsMod     bool   `json:"isMod,omitempty" table:"-"`
	Type      string `json:"type,omitempty" table:"-"`

	// The overview repeats the name under a second key and adds the account's
	// age, which is the only place on the hub a person's join date appears.
	Handle              string    `json:"user,omitempty" table:"-"`
	CreatedAt           time.Time `json:"createdAt,omitzero" table:"-"`
	PrimaryOrgAvatarURL string    `json:"primaryOrgAvatarUrl,omitempty" table:"-"`
	// IsFollowing is relative to the token making the request, so it is empty
	// for an anonymous read and true only for the caller's own following list.
	IsFollowing bool `json:"isFollowing,omitempty" table:"-"`

	NumModels        int `json:"numModels" table:"models"`
	NumDatasets      int `json:"numDatasets" table:"datasets"`
	NumSpaces        int `json:"numSpaces" table:"spaces"`
	NumKernels       int `json:"numKernels" table:"-"`
	NumBuckets       int `json:"numBuckets,omitempty" table:"-"`
	NumPapers        int `json:"numPapers" table:"-"`
	NumDiscussions   int `json:"numDiscussions" table:"-"`
	NumUpvotes       int `json:"numUpvotes" table:"-"`
	NumLikes         int `json:"numLikes" table:"-"`
	NumFollowers     int `json:"numFollowers" table:"followers"`
	NumFollowing     int `json:"numFollowing" table:"-"`
	NumFollowingOrgs int `json:"numFollowingOrgs,omitempty" table:"-"`

	Orgs []UserRef `json:"orgs,omitempty" table:"-"`

	// Page-derived. The profile carries the first page of each list inline, so a
	// deep fetch of a namespace answers what three list calls would.
	CommunityScore int          `json:"communityScore,omitempty" table:"-"`
	Activities     []Activity   `json:"activities,omitempty" table:"-"`
	BlogPosts      []BlogRef    `json:"blogPosts,omitempty" table:"-"`
	TotalBlogPosts int          `json:"totalBlogPosts,omitempty" table:"-"`
	Models         []Model      `json:"models,omitempty" table:"-"`
	Datasets       []Dataset    `json:"datasets,omitempty" table:"-"`
	Spaces         []Space      `json:"spaces,omitempty" table:"-"`
	Collections    []Collection `json:"collections,omitempty" table:"-"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (u *User) UnmarshalJSON(b []byte) error {
	type raw User
	return decodeExtra(b, (*raw)(u), &u.Extra)
}

// Org is a hub organisation.
type Org struct {
	Meta

	Name         string `json:"name" table:"name"`
	ObjectID     string `json:"_id,omitempty" table:"-"`
	Fullname     string `json:"fullname,omitempty" table:"fullname"`
	Details      string `json:"details,omitempty" table:"-"`
	AvatarURL    string `json:"avatarUrl,omitempty" table:"-"`
	IsVerified   bool   `json:"isVerified,omitempty" table:"-"`
	IsEnterprise bool   `json:"isEnterprise,omitempty" table:"-"`
	Plan         string `json:"plan,omitempty" table:"-"`
	Type         string `json:"type,omitempty" table:"-"`

	// IsFollowing is relative to the token making the request.
	IsFollowing bool `json:"isFollowing,omitempty" table:"-"`

	NumUsers     int `json:"numUsers" table:"members"`
	NumModels    int `json:"numModels" table:"models"`
	NumDatasets  int `json:"numDatasets" table:"datasets"`
	NumSpaces    int `json:"numSpaces" table:"spaces"`
	NumKernels   int `json:"numKernels" table:"-"`
	NumBuckets   int `json:"numBuckets,omitempty" table:"-"`
	NumPapers    int `json:"numPapers" table:"-"`
	NumFollowers int `json:"numFollowers" table:"followers"`

	// Page-derived. The org page is the single biggest win in the tool: one
	// request replaces the overview, the member list, a follower sample, three
	// repo list calls, a collections call, and a papers call.
	Card            string       `json:"card,omitempty" table:"-"`
	Members         []UserRef    `json:"members,omitempty" table:"-"`
	SampleFollowers []UserRef    `json:"sampleFollowers,omitempty" table:"-"`
	Models          []Model      `json:"models,omitempty" table:"-"`
	Datasets        []Dataset    `json:"datasets,omitempty" table:"-"`
	Spaces          []Space      `json:"spaces,omitempty" table:"-"`
	Collections     []Collection `json:"collections,omitempty" table:"-"`
	Papers          []Paper      `json:"papers,omitempty" table:"-"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (o *Org) UnmarshalJSON(b []byte) error {
	type raw Org
	return decodeExtra(b, (*raw)(o), &o.Extra)
}

// UserRef is the compact namespace object embedded everywhere: as a liker, a
// follower, a member, a commit author, a discussion author, a repo owner.
type UserRef struct {
	Name          string `json:"name" table:"name"`
	ObjectID      string `json:"_id,omitempty" table:"-"`
	Fullname      string `json:"fullname,omitempty" table:"fullname"`
	Type          string `json:"type,omitempty" table:"type"`
	AvatarURL     string `json:"avatarUrl,omitempty" table:"-"`
	IsPro         bool   `json:"isPro,omitempty" table:"-"`
	IsHf          bool   `json:"isHf,omitempty" table:"-"`
	IsHfAdmin     bool   `json:"isHfAdmin,omitempty" table:"-"`
	IsMod         bool   `json:"isMod,omitempty" table:"-"`
	IsEnterprise  bool   `json:"isEnterprise,omitempty" table:"-"`
	Plan          string `json:"plan,omitempty" table:"-"`
	FollowerCount int    `json:"followerCount,omitempty" table:"followers"`

	// IsUserFollowing is relative to the token making the request.
	IsUserFollowing bool `json:"isUserFollowing,omitempty" table:"-"`

	URI string `json:"uri,omitempty" table:"-"`
	URL string `json:"url,omitempty" table:"-,url"`
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

	Name string `json:"name" table:"name"`
	// Whoami is the one endpoint that names the object id "id" rather than "_id".
	ObjectID  string          `json:"id,omitempty" table:"-"`
	Fullname  string          `json:"fullname,omitempty" table:"fullname"`
	Email     string          `json:"email,omitempty" table:"email"`
	Type      string          `json:"type,omitempty" table:"type"`
	IsPro     bool            `json:"isPro,omitempty" table:"pro"`
	CanPay    bool            `json:"canPay,omitempty" table:"-"`
	AvatarURL string          `json:"avatarUrl,omitempty" table:"-"`
	Orgs      []UserRef       `json:"orgs,omitempty" table:"-"`
	Auth      json.RawMessage `json:"auth,omitempty" table:"-"`
}

// UnmarshalJSON decodes the known fields and sweeps the rest into Extra.
func (w *Whoami) UnmarshalJSON(b []byte) error {
	type raw Whoami
	return decodeExtra(b, (*raw)(w), &w.Extra)
}
