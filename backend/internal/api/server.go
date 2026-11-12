// Package api implements the JSON HTTP API consumed by the web frontend.
package api

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/config"
	"github.com/sysvo/knot/internal/mail"
	"github.com/sysvo/knot/internal/markdown"
	"github.com/sysvo/knot/internal/model"
	"github.com/sysvo/knot/internal/ratelimit"
	"github.com/sysvo/knot/internal/store"
)

// Version is set at build time with -ldflags.
var Version = "dev"

// Server wires configuration, storage and HTTP handlers together.
type Server struct {
	cfg     *config.Config
	store   *store.Store
	log     *slog.Logger
	md      *markdown.Renderer
	mail    *mail.Mailer
	limiter *ratelimit.Limiter
	started time.Time
}

// New builds a Server.
func New(cfg *config.Config, st *store.Store, logger *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		store:   st,
		log:     logger,
		md:      markdown.New(),
		mail:    mail.New(cfg.SMTP, logger),
		limiter: ratelimit.New(),
		started: time.Now(),
	}
}

// Handler returns the full HTTP handler, middleware included.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// ---- public ----
	mux.HandleFunc("GET /api/v1/health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/meta", s.handleMeta)

	// ---- authentication ----
	mux.HandleFunc("POST /api/v1/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/v1/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/v1/auth/me", s.handleMe)
	mux.HandleFunc("POST /api/v1/auth/magic", s.handleMagicRequest)
	mux.HandleFunc("POST /api/v1/auth/magic/consume", s.handleMagicConsume)
	mux.HandleFunc("POST /api/v1/auth/verify", s.handleVerifyEmail)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/start", s.handleOAuthStart)
	mux.HandleFunc("GET /api/v1/auth/oauth/{provider}/callback", s.handleOAuthCallback)

	// ---- account ----
	mux.HandleFunc("PATCH /api/v1/me", s.handleUpdateProfile)
	mux.HandleFunc("POST /api/v1/me/password", s.handleChangePassword)
	mux.HandleFunc("POST /api/v1/me/email", s.handleChangeEmail)
	mux.HandleFunc("POST /api/v1/me/sign-out-others", s.handleSignOutOthers)
	mux.HandleFunc("POST /api/v1/uploads", s.handleUpload)

	// ---- reading ----
	mux.HandleFunc("GET /api/v1/boards", s.handleListBoards)
	mux.HandleFunc("GET /api/v1/boards/{slug}", s.handleGetBoard)
	mux.HandleFunc("GET /api/v1/boards/{slug}/threads", s.handleListBoardThreads)
	mux.HandleFunc("GET /api/v1/threads", s.handleListThreads)
	mux.HandleFunc("GET /api/v1/threads/{id}", s.handleGetThread)
	mux.HandleFunc("GET /api/v1/posts/{id}", s.handleGetPost)
	mux.HandleFunc("GET /api/v1/search", s.handleSearch)
	mux.HandleFunc("GET /api/v1/users/{username}", s.handleGetUser)
	mux.HandleFunc("GET /api/v1/users/{username}/posts", s.handleUserPosts)

	// ---- writing ----
	mux.HandleFunc("POST /api/v1/boards/{slug}/threads", s.handleCreateThread)
	mux.HandleFunc("PATCH /api/v1/threads/{id}", s.handleUpdateThread)
	mux.HandleFunc("DELETE /api/v1/threads/{id}", s.handleDeleteThread)
	mux.HandleFunc("POST /api/v1/threads/{id}/posts", s.handleCreatePost)
	mux.HandleFunc("PATCH /api/v1/posts/{id}", s.handleUpdatePost)
	mux.HandleFunc("DELETE /api/v1/posts/{id}", s.handleDeletePost)
	mux.HandleFunc("POST /api/v1/posts/{id}/vote", s.handleVote)
	mux.HandleFunc("POST /api/v1/posts/{id}/report", s.handleReport)

	// ---- notifications ----
	mux.HandleFunc("GET /api/v1/notifications", s.handleListNotifications)
	mux.HandleFunc("POST /api/v1/notifications/read", s.handleReadNotifications)
	mux.HandleFunc("DELETE /api/v1/notifications", s.handleDeleteNotifications)

	// ---- moderation and administration ----
	mux.HandleFunc("POST /api/v1/threads/{id}/pin", s.handleTogglePin)
	mux.HandleFunc("POST /api/v1/threads/{id}/lock", s.handleToggleLock)
	mux.HandleFunc("GET /api/v1/admin/stats", s.handleAdminStats)
	mux.HandleFunc("GET /api/v1/admin/users", s.handleAdminUsers)
	mux.HandleFunc("PATCH /api/v1/admin/users/{id}", s.handleAdminUpdateUser)
	mux.HandleFunc("GET /api/v1/admin/reports", s.handleAdminReports)
	mux.HandleFunc("POST /api/v1/admin/reports/{id}/resolve", s.handleAdminResolveReport)
	mux.HandleFunc("GET /api/v1/admin/pending", s.handleAdminPending)
	mux.HandleFunc("POST /api/v1/admin/threads/{id}/approve", s.handleAdminApproveThread)
	mux.HandleFunc("POST /api/v1/admin/posts/{id}/approve", s.handleAdminApprovePost)
	mux.HandleFunc("GET /api/v1/admin/settings", s.handleAdminGetSettings)
	mux.HandleFunc("PATCH /api/v1/admin/settings", s.handleAdminSetSettings)
	mux.HandleFunc("POST /api/v1/admin/boards", s.handleAdminCreateBoard)
	mux.HandleFunc("PATCH /api/v1/admin/boards/{id}", s.handleAdminUpdateBoard)
	mux.HandleFunc("DELETE /api/v1/admin/boards/{id}", s.handleAdminDeleteBoard)
	mux.HandleFunc("GET /api/v1/admin/audit", s.handleAdminAudit)

	// ---- uploaded files ----
	mux.HandleFunc("GET /uploads/{name}", s.handleServeUpload)

	// Anything else under /api is a 404 in JSON rather than HTML.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeError(w, http.StatusNotFound, "not_found", "no such endpoint")
	})

	return s.recoverer(
		s.secureHeaders(
			s.originCheck(
				s.loadSession(
					s.logging(mux)))))
}

// Run starts the HTTP server and blocks until ctx is cancelled, then shuts down
// gracefully.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:              s.cfg.Addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go s.background(ctx)

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

// Bootstrap creates the first administrator from the environment when the
// database has no users yet. It never modifies an existing account.
func (s *Server) Bootstrap(ctx context.Context) error {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		return err
	}
	if n > 0 || s.cfg.BootstrapAdmin == "" || s.cfg.BootstrapPassword == "" {
		return nil
	}

	hash, err := auth.HashPassword(s.cfg.BootstrapPassword)
	if err != nil {
		return err
	}
	if _, err := s.store.CreateAccount(ctx, s.cfg.BootstrapAdmin, s.cfg.BootstrapEmail, hash, model.RoleAdmin, s.cfg.BootstrapEmail != ""); err != nil {
		return err
	}
	s.log.Info("created initial administrator", "username", s.cfg.BootstrapAdmin)
	return nil
}

// background runs periodic housekeeping until ctx is cancelled.
func (s *Server) background(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	s.housekeeping(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.housekeeping(ctx)
		}
	}
}

func (s *Server) housekeeping(ctx context.Context) {
	if err := s.store.PurgeExpiredSessions(ctx); err != nil {
		s.log.Warn("purge expired sessions", "err", err)
	}
	s.limiter.Purge(time.Hour)
}

// handleServeUpload serves a stored attachment from disk. Files are served with
// a long cache lifetime because their names are random and never reused.
func (s *Server) handleServeUpload(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid file name")
		return
	}
	path := filepath.Join(s.cfg.UploadDir, name)
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "file not found")
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	http.ServeFile(w, r, path)
}

// handleHealth is used by container health checks and uptime monitors.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.store.HealthCheck(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"version": Version,
		"uptime":  int64(time.Since(s.started).Seconds()),
	})
}

// clientIP returns the caller's address, honouring X-Forwarded-For only when
// the deployment says a reverse proxy is in front.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if idx := strings.IndexByte(xff, ','); idx > 0 {
				return strings.TrimSpace(xff[:idx])
			}
			return strings.TrimSpace(xff)
		}
		if rip := r.Header.Get("X-Real-Ip"); rip != "" {
			return rip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
