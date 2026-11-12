package api

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/model"
	"github.com/sysvo/knot/internal/policy"
)

// spamRules reads the thresholds from the site settings. A settings failure
// falls back to the defaults rather than disabling the protection, because the
// fallback is stricter than "no rules".
func (s *Server) spamRules(ctx context.Context) policy.Rules {
	all, err := s.store.Settings(ctx)
	if err != nil {
		s.log.Error("read spam settings", "err", err)
		return policy.Defaults()
	}
	return policy.FromSettings(all)
}

// assess works out what this account is currently allowed to do.
func (s *Server) assess(ctx context.Context, u *model.User) (policy.Verdict, error) {
	published, err := s.store.UserPostCount(ctx, u.ID)
	if err != nil {
		return policy.Verdict{}, err
	}
	return policy.Assess(policy.Account{
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		Published: published,
	}, s.spamRules(ctx), time.Now()), nil
}

// allowedLinkHosts are the hosts a post may link to without being treated as
// advertising: this forum itself.
func (s *Server) allowedLinkHosts() []string {
	u, err := url.Parse(s.cfg.PublicURL)
	if err != nil || u.Hostname() == "" {
		return nil
	}
	return []string{u.Hostname()}
}

// checkQuota enforces the daily posting limit for accounts that are not yet
// established. It writes the response and reports whether to continue.
func (s *Server) checkQuota(w http.ResponseWriter, r *http.Request, u *model.User, v policy.Verdict) bool {
	if v.DailyLimit <= 0 {
		return true
	}
	since := time.Now().Add(-24 * time.Hour).Unix()
	recent, err := s.store.PostsInLastDay(r.Context(), u.ID, since)
	if err != nil {
		s.log.Error("count recent posts", "err", err, "user", u.ID)
		writeError(w, http.StatusInternalServerError, "internal", "could not check your posting allowance")
		return false
	}
	if recent >= int64(v.DailyLimit) {
		w.Header().Set("Retry-After", "3600")
		writeError(w, http.StatusTooManyRequests, "daily_limit",
			fmt.Sprintf("new accounts can post %d times a day. This limit lifts once your account is a few days old with a few posts behind it.", v.DailyLimit))
		return false
	}
	return true
}

// checkLinks blocks off-site links from accounts that have not earned them.
func (s *Server) checkLinks(w http.ResponseWriter, r *http.Request, html string, v policy.Verdict) bool {
	if v.MayPostLinks {
		return true
	}

	hosts := policy.ExternalLinks(html, s.allowedLinkHosts()...)
	if len(hosts) == 0 {
		return true
	}

	rules := s.spamRules(r.Context())
	writeError(w, http.StatusForbidden, "links_not_allowed", fmt.Sprintf(
		"new accounts cannot post links to other sites yet. Links unlock after %d posts over %d days; you can post this without the link, or ask a moderator.",
		rules.LinksRequirePosts, rules.LinksRequireDays))
	s.log.Info("blocked links from new account", "user", u(r), "hosts", strings.Join(hosts, ","))
	return false
}

// u returns the signed-in user's id for logging, or 0.
func u(r *http.Request) int64 {
	if user := currentUser(r); user != nil {
		return user.ID
	}
	return 0
}

// notifyPostPublished tells the thread author and anyone mentioned. It is called
// when a post actually becomes visible, which is either immediately or when a
// moderator approves it — never while it is held for review.
func (s *Server) notifyPostPublished(ctx context.Context, actor *model.User, post *model.Post) {
	thread, err := s.store.ThreadByID(ctx, post.ThreadID)
	if err != nil {
		s.log.Error("notify: load thread", "err", err, "thread", post.ThreadID)
		return
	}
	if thread.Author != nil {
		_ = s.store.Notify(ctx, thread.Author.ID, actor.ID, "reply", thread.ID, post.ID)
	}
	s.notifyMentions(ctx, actor, post.BodyMD, thread.ID, post.ID)
}
