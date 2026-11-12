// Package policy holds the rules that decide whether an account is trusted
// enough to post links, and whether its next post has to be reviewed first.
//
// It is deliberately free of HTTP and SQL: the rules are the part most likely to
// be argued about, so they are pure functions with their own tests.
package policy

import (
	"strconv"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/model"
)

// Setting keys owned by this package.
const (
	KeyLinksRequirePosts = "spam_links_require_posts"
	KeyLinksRequireDays  = "spam_links_require_days"
	KeyNewPostsPerDay    = "spam_new_posts_per_day"
	KeyApproveFirstPosts = "spam_approve_first_posts"
	KeyBlockedDomains    = "spam_blocked_email_domains"
)

// Rules are the thresholds for a new account.
type Rules struct {
	// LinksRequirePosts and LinksRequireDays describe how an account earns the
	// right to post links: it needs this many published posts and this many days
	// since joining, whichever is met last.
	LinksRequirePosts int
	LinksRequireDays  int
	// NewPostsPerDay caps daily posting for accounts that are not yet
	// established. Zero means no limit.
	NewPostsPerDay int
	// ApproveFirstPosts holds this many of a new account's first posts for
	// review. Zero disables the queue entirely.
	ApproveFirstPosts int
	// BlockedDomains are e-mail domains refused at registration.
	BlockedDomains []string
}

// Defaults are used when a setting is missing or unreadable. They are the
// values the seed migration writes.
func Defaults() Rules {
	return Rules{
		LinksRequirePosts: 3,
		LinksRequireDays:  2,
		NewPostsPerDay:    20,
		ApproveFirstPosts: 0,
	}
}

// FromSettings builds Rules from the site settings map, falling back to the
// defaults for anything missing and clamping nonsense to something sane.
func FromSettings(settings map[string]string) Rules {
	r := Defaults()
	r.LinksRequirePosts = clamp(intValue(settings, KeyLinksRequirePosts, r.LinksRequirePosts), 0, 1000)
	r.LinksRequireDays = clamp(intValue(settings, KeyLinksRequireDays, r.LinksRequireDays), 0, 365)
	r.NewPostsPerDay = clamp(intValue(settings, KeyNewPostsPerDay, r.NewPostsPerDay), 0, 10000)
	r.ApproveFirstPosts = clamp(intValue(settings, KeyApproveFirstPosts, r.ApproveFirstPosts), 0, 100)
	r.BlockedDomains = splitDomains(settings[KeyBlockedDomains])
	return r
}

func intValue(m map[string]string, key string, def int) int {
	raw, ok := m[key]
	if !ok || strings.TrimSpace(raw) == "" {
		return def
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return def
	}
	return n
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// splitDomains accepts a comma-, space- or newline-separated list, because
// admins will paste whichever they have.
func splitDomains(raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == ' ' || r == '\t' || r == ';'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		d := strings.ToLower(strings.Trim(strings.TrimPrefix(strings.TrimSpace(f), "@"), "."))
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// Account is the input to the decision: who is posting, and how much they have
// already published.
type Account struct {
	Role      string
	CreatedAt int64 // unix seconds, when the account was created
	Published int64 // published, undeleted posts
}

// Verdict is the decision for one post.
type Verdict struct {
	Established   bool
	MayPostLinks  bool
	NeedsApproval bool
	// DailyLimit is the number of posts per rolling day, or 0 for no limit.
	DailyLimit int
}

// Assess decides what an account is allowed to do right now.
func Assess(acc Account, rules Rules, now time.Time) Verdict {
	age := time.Duration(0)
	if acc.CreatedAt > 0 {
		age = now.Sub(time.Unix(acc.CreatedAt, 0))
	}

	// Staff are never treated as new: they are the people clearing the queue.
	staff := acc.Role == model.RoleAdmin || acc.Role == model.RoleModerator

	oldEnough := age >= time.Duration(rules.LinksRequireDays)*24*time.Hour
	postedEnough := acc.Published >= int64(rules.LinksRequirePosts)

	return Verdict{
		Established:   staff || (oldEnough && postedEnough),
		MayPostLinks:  staff || (oldEnough && postedEnough),
		NeedsApproval: !staff && rules.ApproveFirstPosts > 0 && acc.Published < int64(rules.ApproveFirstPosts),
		DailyLimit:    dailyLimit(staff, oldEnough && postedEnough, rules.NewPostsPerDay),
	}
}

func dailyLimit(staff, established bool, configured int) int {
	if staff || established {
		return 0
	}
	return configured
}

// EmailBlocked reports whether an address belongs to a blocked domain. A domain
// matches itself and its subdomains, so blocking "mailinator.com" also blocks
// "eu.mailinator.com".
func (r Rules) EmailBlocked(email string) bool {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return false
	}
	domain := strings.ToLower(strings.TrimSuffix(email[at+1:], "."))
	for _, blocked := range r.BlockedDomains {
		if domain == blocked || strings.HasSuffix(domain, "."+blocked) {
			return true
		}
	}
	return false
}
