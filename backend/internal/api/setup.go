package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/policy"
	"github.com/sysvo/knot/internal/store"
)

// needsSetup reports whether the instance has no accounts at all.
//
// Everything that would create a first account consults this, because setup is
// the only thing allowed to create one: a stranger who registered before the
// administrator did would leave an instance with no admin and, since setup needs
// an empty table, no way to run it.
func (s *Server) needsSetup(ctx context.Context) bool {
	n, err := s.store.CountUsers(ctx)
	if err != nil {
		s.log.Warn("count accounts", "err", err)
		return false
	}
	return n == 0
}

// handleSetupStatus reports whether the instance still needs an administrator.
//
// "No accounts exist" is the whole condition: there is no flag to get out of
// step with the data, and deleting every account legitimately reopens setup.
func (s *Server) handleSetupStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.CountUsers(r.Context())
	if err != nil {
		writeStoreError(s, w, r, err, "could not read the account table")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"needs_setup": n == 0,
		"site_name":   s.getSetting(r.Context(), settingSiteName, "knot"),
	})
}

type setupRequest struct {
	SiteName string `json:"site_name"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
	// Optional mail relay, so the whole first run is one form.
	SMTPHost string `json:"smtp_host"`
	SMTPPort string `json:"smtp_port"`
	SMTPFrom string `json:"smtp_from"`
}

// handleSetup creates the first administrator and the site settings.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, "setup", 10, 3) {
		return
	}

	var req setupRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	siteName := strings.TrimSpace(req.SiteName)

	if err := auth.ValidateUsername(req.Username); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_username", err.Error())
		return
	}
	if req.Email != "" {
		if err := auth.ValidateEmail(req.Email); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
			return
		}
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}
	if siteName != "" && len([]rune(siteName)) > 40 {
		writeError(w, http.StatusBadRequest, "invalid", "the site name must be 40 characters or fewer")
		return
	}
	if req.SMTPPort != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(req.SMTPPort)); err != nil || n < 1 || n > 65535 {
			writeError(w, http.StatusBadRequest, "invalid", "the SMTP port must be a number between 1 and 65535")
			return
		}
	}
	if req.SMTPFrom != "" {
		if err := auth.ValidateEmail(strings.TrimSpace(req.SMTPFrom)); err != nil {
			writeError(w, http.StatusBadRequest, "invalid", "sender address: "+err.Error())
			return
		}
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create the account")
		return
	}

	acct, err := s.store.CreateFirstAdmin(r.Context(), req.Username, req.Email, hash, req.Email == "")
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			writeError(w, http.StatusConflict, "already_set_up",
				"this forum already has an account, so setup is closed")
			return
		}
		writeStoreError(s, w, r, err, "could not create the administrator")
		return
	}

	settings := map[string]string{policy.KeyApproveFirstPosts: "0"}
	if siteName != "" {
		settings[settingSiteName] = siteName
	}
	if host := strings.TrimSpace(req.SMTPHost); host != "" {
		settings[settingSMTPHost] = host
		if port := strings.TrimSpace(req.SMTPPort); port != "" {
			settings[settingSMTPPort] = port
		}
		if from := strings.TrimSpace(req.SMTPFrom); from != "" {
			settings[settingSMTPFrom] = from
		}
	}
	if err := s.store.SetSettings(r.Context(), settings); err != nil {
		s.log.Error("save setup settings", "err", err)
	}

	_ = s.store.Log(r.Context(), acct.User.ID, "user.setup", "user", acct.User.ID, "")

	// Sign the new administrator in: they have just proved they hold the
	// credentials, and making them log in again immediately is theatre.
	if err := s.startSession(r.Context(), w, r, acct.User.ID); err != nil {
		s.log.Error("start session after setup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "account created, please sign in")
		return
	}

	writeJSON(w, http.StatusCreated, acct.Self())
}
