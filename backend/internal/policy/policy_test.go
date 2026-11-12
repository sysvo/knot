package policy_test

import (
	"testing"
	"time"

	"github.com/sysvo/knot/internal/model"
	"github.com/sysvo/knot/internal/policy"
)

var now = time.Unix(1_800_000_000, 0)

func hoursAgo(h int) int64 { return now.Add(-time.Duration(h) * time.Hour).Unix() }

// TestNewAccountCannotPostLinks is the rule that keeps a fresh spam account from
// advertising anything: no links until it has posted and aged a little.
func TestNewAccountCannotPostLinks(t *testing.T) {
	rules := policy.Rules{LinksRequirePosts: 3, LinksRequireDays: 2, NewPostsPerDay: 20}

	fresh := policy.Account{CreatedAt: hoursAgo(1), Published: 0}
	v := policy.Assess(fresh, rules, now)
	if v.Established || v.MayPostLinks {
		t.Fatalf("a one-hour-old account with no posts is allowed links: %+v", v)
	}
	if v.NeedsApproval {
		t.Fatal("approval should be off when the rule is zero")
	}
	if v.DailyLimit != 20 {
		t.Fatalf("DailyLimit = %d, want 20", v.DailyLimit)
	}
}

// TestEstablishmentNeedsBothCountAndAge proves the thresholds are conjunctive:
// spamming ten posts in an hour must not unlock links.
func TestEstablishmentNeedsBothCountAndAge(t *testing.T) {
	rules := policy.Rules{LinksRequirePosts: 3, LinksRequireDays: 2, NewPostsPerDay: 20}

	burst := policy.Account{CreatedAt: hoursAgo(1), Published: 50}
	if v := policy.Assess(burst, rules, now); v.MayPostLinks {
		t.Fatal("50 posts in one hour unlocked links")
	}

	old := policy.Account{CreatedAt: hoursAgo(24 * 10), Published: 0}
	if v := policy.Assess(old, rules, now); v.MayPostLinks {
		t.Fatal("a ten-day-old account with no posts unlocked links")
	}

	both := policy.Account{CreatedAt: hoursAgo(24 * 3), Published: 4}
	v := policy.Assess(both, rules, now)
	if !v.MayPostLinks || !v.Established {
		t.Fatalf("an established account is still restricted: %+v", v)
	}
	if v.DailyLimit != 0 {
		t.Fatalf("established account has a daily limit of %d", v.DailyLimit)
	}
}

// TestZeroThresholdsAllowEverything lets an admin turn the rules off entirely.
func TestZeroThresholdsAllowEverything(t *testing.T) {
	rules := policy.Rules{LinksRequirePosts: 0, LinksRequireDays: 0, NewPostsPerDay: 0}
	v := policy.Assess(policy.Account{CreatedAt: hoursAgo(1)}, rules, now)

	if !v.MayPostLinks {
		t.Fatal("zero thresholds still blocked links")
	}
	if v.DailyLimit != 0 {
		t.Fatalf("zero threshold produced a limit of %d", v.DailyLimit)
	}
}

func TestStaffAreNeverRestricted(t *testing.T) {
	rules := policy.Rules{LinksRequirePosts: 3, LinksRequireDays: 2, NewPostsPerDay: 5, ApproveFirstPosts: 3}

	for _, role := range []string{model.RoleAdmin, model.RoleModerator} {
		v := policy.Assess(policy.Account{Role: role, CreatedAt: hoursAgo(1)}, rules, now)
		if v.NeedsApproval || !v.MayPostLinks || v.DailyLimit != 0 {
			t.Fatalf("%s was restricted: %+v", role, v)
		}
	}

	// A member with the same history is not so lucky.
	v := policy.Assess(policy.Account{Role: model.RoleMember, CreatedAt: hoursAgo(1)}, rules, now)
	if !v.NeedsApproval {
		t.Fatal("a new member's first posts should need approval")
	}
}

func TestApproveFirstPostsCountsPublishedPosts(t *testing.T) {
	rules := policy.Rules{ApproveFirstPosts: 2}

	if v := policy.Assess(policy.Account{Published: 0}, rules, now); !v.NeedsApproval {
		t.Fatal("first post should need approval")
	}
	if v := policy.Assess(policy.Account{Published: 1}, rules, now); !v.NeedsApproval {
		t.Fatal("second post should need approval")
	}
	if v := policy.Assess(policy.Account{Published: 2}, rules, now); v.NeedsApproval {
		t.Fatal("third post should be published directly")
	}
}

func TestFromSettingsParsesAndClamps(t *testing.T) {
	rules := policy.FromSettings(map[string]string{
		policy.KeyLinksRequirePosts: "not a number",
		policy.KeyLinksRequireDays:  "-5",
		policy.KeyNewPostsPerDay:    "999999",
		policy.KeyApproveFirstPosts: "2",
	})

	if rules.LinksRequirePosts != policy.Defaults().LinksRequirePosts {
		t.Errorf("garbage should fall back to the default, got %d", rules.LinksRequirePosts)
	}
	if rules.LinksRequireDays != 0 {
		t.Errorf("negative days should clamp to 0, got %d", rules.LinksRequireDays)
	}
	if rules.NewPostsPerDay > 10000 {
		t.Errorf("daily limit was not clamped: %d", rules.NewPostsPerDay)
	}
	if rules.ApproveFirstPosts != 2 {
		t.Errorf("ApproveFirstPosts = %d, want 2", rules.ApproveFirstPosts)
	}
}

func TestEmailBlocklistMatchesSubdomains(t *testing.T) {
	rules := policy.FromSettings(map[string]string{
		policy.KeyBlockedDomains: "@mailinator.com, trashmail.net\ndisposable.example",
	})

	blocked := []string{
		"a@mailinator.com",
		"A@MAILINATOR.COM",
		"b@eu.mailinator.com",
		"c@trashmail.net",
		"d@sub.disposable.example",
	}
	for _, addr := range blocked {
		if !rules.EmailBlocked(addr) {
			t.Errorf("%s should be blocked", addr)
		}
	}

	allowed := []string{
		"a@example.com",
		"a@notmailinator.com",   // suffix match must respect the dot
		"a@mailinator.com.evil", // the blocked name must not appear as a prefix
		"",
		"no-at-sign",
	}
	for _, addr := range allowed {
		if rules.EmailBlocked(addr) {
			t.Errorf("%s should be allowed", addr)
		}
	}
}
