// Package model holds the types that are shared between the store, the HTTP
// layer and (via JSON) the frontend.
package model

const (
	RoleMember    = "member"
	RoleModerator = "moderator"
	RoleAdmin     = "admin"

	StatusActive = "active"
	StatusBanned = "banned"
)

// User is the public view of an account. It is safe to show to anyone.
type User struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
}

// Name is the name to show in the interface.
func (u *User) Name() string {
	if u == nil {
		return "[deleted]"
	}
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

// CanModerate reports whether the user may act on other people's content.
func (u *User) CanModerate() bool {
	return u != nil && (u.Role == RoleModerator || u.Role == RoleAdmin)
}

// IsAdmin reports whether the user may change site settings and roles.
func (u *User) IsAdmin() bool {
	return u != nil && u.Role == RoleAdmin
}

// Self is the private view of the signed-in account.
type Self struct {
	User
	Email         string   `json:"email"`
	EmailVerified bool     `json:"email_verified"`
	HasPassword   bool     `json:"has_password"`
	Providers     []string `json:"providers"`
}

// Board is a category of threads.
type Board struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Position    int    `json:"position"`
	CreatedAt   int64  `json:"created_at"`
	ThreadCount int64  `json:"thread_count"`
	PostCount   int64  `json:"post_count"`
	LastPostAt  int64  `json:"last_post_at"`
}

// Thread is a topic: a title plus an ordered list of posts.
type Thread struct {
	ID         int64  `json:"id"`
	BoardID    int64  `json:"board_id"`
	BoardSlug  string `json:"board_slug"`
	BoardName  string `json:"board_name"`
	Title      string `json:"title"`
	Pinned     bool   `json:"pinned"`
	Locked     bool   `json:"locked"`
	Deleted    bool   `json:"deleted"`
	Author     *User  `json:"author"`
	LastAuthor *User  `json:"last_author"`
	Excerpt    string `json:"excerpt"`
	PostCount  int64  `json:"post_count"`
	ViewCount  int64  `json:"view_count"`
	CreatedAt  int64  `json:"created_at"`
	LastPostAt int64  `json:"last_post_at"`
}

// Post is a single message inside a thread.
type Post struct {
	ID        int64  `json:"id"`
	ThreadID  int64  `json:"thread_id"`
	Author    *User  `json:"author"`
	BodyMD    string `json:"body_md"`
	BodyHTML  string `json:"body_html"`
	Score     int64  `json:"score"`
	MyVote    int    `json:"my_vote"`
	Deleted   bool   `json:"deleted"`
	CreatedAt int64  `json:"created_at"`
	EditedAt  *int64 `json:"edited_at"`
	Hidden    bool   `json:"hidden"`
}

// Notification is an event the user should see.
type Notification struct {
	ID          int64  `json:"id"`
	Kind        string `json:"kind"`
	Actor       *User  `json:"actor"`
	ThreadID    int64  `json:"thread_id"`
	ThreadTitle string `json:"thread_title"`
	PostID      int64  `json:"post_id"`
	Excerpt     string `json:"excerpt"`
	Read        bool   `json:"read"`
	CreatedAt   int64  `json:"created_at"`
}

// Report is a user's complaint about a post or thread.
type Report struct {
	ID          int64  `json:"id"`
	Reporter    *User  `json:"reporter"`
	PostID      *int64 `json:"post_id"`
	ThreadID    *int64 `json:"thread_id"`
	ThreadTitle string `json:"thread_title"`
	Excerpt     string `json:"excerpt"`
	Reason      string `json:"reason"`
	Details     string `json:"details"`
	Status      string `json:"status"`
	CreatedAt   int64  `json:"created_at"`
}

// Upload is a stored attachment.
type Upload struct {
	ID          int64  `json:"id"`
	URL         string `json:"url"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
}

// SearchHit is one result from the full-text index.
type SearchHit struct {
	ThreadID   int64  `json:"thread_id"`
	PostID     int64  `json:"post_id"`
	Title      string `json:"title"`
	BoardSlug  string `json:"board_slug"`
	BoardName  string `json:"board_name"`
	Snippet    string `json:"snippet"`
	Author     string `json:"author"`
	LastPostAt int64  `json:"last_post_at"`
	CreatedAt  int64  `json:"created_at"`
}

// Page describes a slice of a larger result set.
type Page struct {
	Number  int   `json:"number"`
	PerPage int   `json:"per_page"`
	Total   int64 `json:"total"`
	HasNext bool  `json:"has_next"`
}
