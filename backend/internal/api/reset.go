package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/model"
	"github.com/sysvo/knot/internal/store"
)

// resetTokenTTL is how long a password-reset link stays valid.
const resetTokenTTL = 60 * time.Minute

// handleResetRequest starts a password reset.
//
// The answer is identical whether or not the address exists, because "no such
// account" is an account-enumeration oracle and a reset form is the easiest
// place to use one.
func (s *Server) handleResetRequest(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, "reset", 10, 5) {
		return
	}
	var req emailRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if err := auth.ValidateEmail(email); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
		return
	}
	if !s.smtpConfig(r.Context()).Enabled() {
		writeError(w, http.StatusNotImplemented, "mail_unavailable",
			"this instance has no mail server configured, so passwords cannot be reset by e-mail")
		return
	}

	acct, err := s.store.AccountByEmail(r.Context(), email)
	if err != nil && err != store.ErrNotFound {
		writeStoreError(s, w, r, err, "account not found")
		return
	}

	if err == nil && acct.User.Status == model.StatusActive {
		s.sendResetMail(r, acct)
	}

	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "if that address belongs to an account, a reset link is on its way",
	})
}

// sendResetMail issues the token and queues the message.
func (s *Server) sendResetMail(r *http.Request, acct *store.Account) {
	raw, hash, err := auth.NewToken()
	if err != nil {
		s.log.Error("create reset token", "err", err)
		return
	}
	expires := time.Now().Add(resetTokenTTL)
	if err := s.store.CreateLoginToken(r.Context(), hash, acct.Email, &acct.User.ID, "reset", expires.Unix()); err != nil {
		s.log.Error("store reset token", "err", err)
		return
	}

	site := s.getSetting(r.Context(), settingSiteName, "knot")
	link := s.cfg.PublicURL + "/auth/reset?token=" + raw
	body := "Someone asked to reset the password for your account at " + site + ".\n\n" +
		link + "\n\nThe link works once and expires in an hour. " +
		"If this was not you, nothing has changed and you can ignore this message.\n"

	s.queueMailTo(r.Context(), acct.Email, "Reset your "+site+" password", body)
}

type resetConfirmRequest struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// handleResetConfirm sets the new password and signs the reader in.
func (s *Server) handleResetConfirm(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, "reset-confirm", 20, 10) {
		return
	}
	var req resetConfirmRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "missing token")
		return
	}

	userID, _, err := s.store.ConsumeLoginToken(r.Context(), auth.HashToken(req.Token), "reset")
	if err != nil {
		writeStoreError(s, w, r, err, "that link is not valid, or has already been used")
		return
	}
	if !userID.Valid {
		writeError(w, http.StatusBadRequest, "bad_token", "that link is not valid")
		return
	}

	acct, err := s.store.AccountByID(r.Context(), userID.Int64)
	if err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	if acct.User.Status == model.StatusBanned {
		writeError(w, http.StatusForbidden, "banned", "this account has been suspended")
		return
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store the password")
		return
	}
	if err := s.store.SetPassword(r.Context(), acct.User.ID, hash); err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}

	// Whoever had the old password keeps nothing: a reset is also the response
	// to "someone else got in".
	if err := s.store.DeleteOtherSessions(r.Context(), acct.User.ID, ""); err != nil {
		s.log.Error("delete sessions after reset", "err", err, "user", acct.User.ID)
	}
	_ = s.store.Log(r.Context(), 0, "user.password.reset", "user", acct.User.ID, "")

	if err := s.startSession(r.Context(), w, r, acct.User.ID); err != nil {
		s.log.Error("start session", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "password changed, please sign in")
		return
	}
	writeJSON(w, http.StatusOK, acct.Self())
}
