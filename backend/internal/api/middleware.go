package api

import (
	"context"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/model"
)

// sessionCookieName is the browser cookie holding the opaque session token.
const sessionCookieName = "knot_session"

type ctxKey int

const (
	ctxKeyUser ctxKey = iota
	ctxKeySessionHash
)

// currentUser returns the signed-in user, or nil for anonymous requests.
func currentUser(r *http.Request) *model.User {
	u, _ := r.Context().Value(ctxKeyUser).(*model.User)
	return u
}

// currentSessionHash returns the hash of the caller's session token.
func currentSessionHash(r *http.Request) string {
	h, _ := r.Context().Value(ctxKeySessionHash).(string)
	return h
}

// loadSession resolves the session cookie into a user. An unknown or expired
// cookie is simply anonymous; it is not an error.
func (s *Server) loadSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || cookie.Value == "" {
			next.ServeHTTP(w, r)
			return
		}

		hash := auth.HashToken(cookie.Value)
		user, err := s.store.SessionUser(r.Context(), hash)
		if err != nil {
			s.log.Error("load session", "err", err)
			next.ServeHTTP(w, r)
			return
		}
		if user == nil {
			// Expired or signed out: clear the stale cookie so the browser stops
			// sending it.
			s.clearSessionCookie(w)
			next.ServeHTTP(w, r)
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyUser, user)
		ctx = context.WithValue(ctx, ctxKeySessionHash, hash)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireAuth rejects anonymous callers.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) == nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "you need to sign in")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireModerator rejects members.
func (s *Server) requireModerator(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := currentUser(r); u == nil || !u.CanModerate() {
			writeError(w, http.StatusForbidden, "forbidden", "moderators only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// requireAdmin rejects everyone but administrators.
func (s *Server) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u := currentUser(r); u == nil || !u.IsAdmin() {
			writeError(w, http.StatusForbidden, "forbidden", "administrators only")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// recoverer turns a panic into a logged 500 instead of a dropped connection.
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("panic recovered",
					"path", r.URL.Path, "panic", rec, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "internal", "something went wrong")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// secureHeaders sets conservative defaults. The API never serves HTML, so these
// are mostly defence in depth for browsers that stray onto it.
func (s *Server) secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// logging records one line per request, skipping health checks.
func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/health" {
			next.ServeHTTP(w, r)
			return
		}
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		userID := int64(0)
		if u := currentUser(r); u != nil {
			userID = u.ID
		}
		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"bytes", rec.bytes,
			"ms", time.Since(start).Milliseconds(),
			"user", userID,
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Write(b []byte) (int, error) {
	n, err := r.ResponseWriter.Write(b)
	r.bytes += n
	return n, err
}

// originCheck blocks cross-site state changes. Browsers always send Origin on
// POST/PATCH/DELETE; a missing header means a non-browser client (curl, tests),
// which is out of scope for CSRF.
func (s *Server) originCheck(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !s.sameOrigin(r) {
			writeError(w, http.StatusForbidden, "bad_origin", "cross-site request rejected")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if strings.EqualFold(u.Host, r.Host) {
		return true
	}
	// Behind a reverse proxy the Host seen by Go can differ from the public one.
	if pub, err := url.Parse(s.cfg.PublicURL); err == nil && strings.EqualFold(u.Host, pub.Host) {
		return true
	}
	return false
}

// allow applies a rate limit and writes 429 when it is exceeded. The key
// combines the caller's identity with the action so that, for example, log-in
// attempts cannot exhaust the budget for posting.
func (s *Server) allow(w http.ResponseWriter, r *http.Request, action string, perMinute float64, burst int) bool {
	who := s.clientIP(r)
	if u := currentUser(r); u != nil {
		who = "u" + strconv.FormatInt(u.ID, 10)
	}
	if s.limiter.Allow(action+":"+who, perMinute/60, burst) {
		return true
	}
	w.Header().Set("Retry-After", "60")
	writeError(w, http.StatusTooManyRequests, "rate_limited", "too many requests, slow down a little")
	return false
}

// ---- cookie helpers ----

func (s *Server) setSessionCookie(w http.ResponseWriter, raw string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    raw,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// startSession creates a session for the user and sets the cookie.
func (s *Server) startSession(ctx context.Context, w http.ResponseWriter, r *http.Request, userID int64) error {
	raw, hash, err := auth.NewToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(ctx, userID, hash, expires.Unix(), r.UserAgent(), s.clientIP(r)); err != nil {
		return err
	}
	s.setSessionCookie(w, raw, expires)
	return nil
}
