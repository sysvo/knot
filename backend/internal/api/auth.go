package api

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/model"
	"github.com/sysvo/knot/internal/store"
)

// Settings keys used by more than one handler.
const (
	settingSiteName         = "site_name"
	settingSiteDescription  = "site_description"
	settingAllowRegister    = "allow_registration"
	settingRequireVerify    = "require_email_verification"
	settingRegistrationNote = "registration_note"
)

// getSetting reads one setting with a fallback.
func (s *Server) getSetting(ctx context.Context, key, def string) string {
	all, err := s.store.Settings(ctx)
	if err != nil {
		s.log.Error("read settings", "err", err)
		return def
	}
	if v, ok := all[key]; ok {
		return v
	}
	return def
}

func (s *Server) settingBool(ctx context.Context, key string, def bool) bool {
	switch strings.ToLower(s.getSetting(ctx, key, "")) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return def
}

// dummyHash is verified when an account does not exist so that a wrong
// username takes as long as a wrong password.
var (
	dummyHashOnce sync.Once
	dummyHash     string
)

func (s *Server) equaliseTiming(password string) {
	dummyHashOnce.Do(func() {
		dummyHash, _ = auth.HashPassword("timing-equalisation-placeholder")
	})
	auth.VerifyPassword(dummyHash, password)
}

// handleMeta returns the public site configuration plus the current user, so
// the frontend can render a complete page header in one request.
func (s *Server) handleMeta(w http.ResponseWriter, r *http.Request) {
	all, err := s.store.Settings(r.Context())
	if err != nil {
		s.log.Error("read settings", "err", err)
		all = map[string]string{}
	}

	providers := make([]string, 0, len(s.cfg.Providers))
	for _, p := range s.cfg.Providers {
		providers = append(providers, p.Name)
	}

	var self *model.Self
	if u := currentUser(r); u != nil {
		if acct, err := s.store.AccountByID(r.Context(), u.ID); err == nil {
			self = acct.Self()
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"site_name":                  valueOr(all, settingSiteName, "knot"),
		"site_description":           valueOr(all, settingSiteDescription, ""),
		"allow_registration":         strings.EqualFold(valueOr(all, settingAllowRegister, "true"), "true"),
		"require_email_verification": strings.EqualFold(valueOr(all, settingRequireVerify, "false"), "true"),
		"registration_note":          valueOr(all, settingRegistrationNote, ""),
		"providers":                  providers,
		"mail_enabled":               s.smtpConfig(r.Context()).Enabled(),
		"max_upload_bytes":           s.cfg.MaxUploadSize,
		"version":                    Version,
		"user":                       self,
	})
}

func valueOr(m map[string]string, key, def string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return def
}

// ---- register / login / logout ----

type registerRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`

	// Website is a honeypot. It is hidden from people and left empty by them;
	// a bot filling in every field gives itself away. The name is deliberately
	// plausible, because obvious names are the ones spam scripts skip.
	Website string `json:"website"`
	// ElapsedMs is how long the form was on screen before submission. A filled
	// form in under a second was not filled in by a person.
	ElapsedMs int `json:"elapsed_ms"`
}

// minFormMillis is the shortest plausible time to fill in a registration form.
const minFormMillis = 1200

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	// A dozen sign-ups in a burst is generous for a household, an office or a
	// small event behind one address, and still nothing to a spam operator. The
	// honeypot and the review queue are what actually stop the bots.
	if !s.allow(w, r, "register", 20, 12) {
		return
	}
	var req registerRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	// A bot that fills the honeypot, or one that submits instantly, gets the
	// same vague failure as any other rejected registration.
	if req.Website != "" || (req.ElapsedMs > 0 && req.ElapsedMs < minFormMillis) {
		s.log.Info("registration blocked: looked automated",
			"ip", s.clientIP(r), "honeypot", req.Website != "", "elapsed_ms", req.ElapsedMs)
		writeError(w, http.StatusBadRequest, "registration_failed", "registration could not be completed")
		return
	}

	if !s.settingBool(r.Context(), settingAllowRegister, true) {
		writeError(w, http.StatusForbidden, "registration_closed", "registration is closed on this forum")
		return
	}

	req.Username = strings.TrimSpace(req.Username)
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))

	if err := auth.ValidateUsername(req.Username); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_username", err.Error())
		return
	}
	if req.Email != "" {
		if err := auth.ValidateEmail(req.Email); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
			return
		}
		if s.spamRules(r.Context()).EmailBlocked(req.Email) {
			writeError(w, http.StatusBadRequest, "email_blocked",
				"that e-mail provider is not accepted here. Please use a different address.")
			return
		}
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		s.log.Error("hash password", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not create the account")
		return
	}

	acct, err := s.store.CreateAccount(r.Context(), req.Username, req.Email, hash, model.RoleMember, req.Email == "")
	if err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}

	// The very first account to exist becomes the administrator.
	if n, err := s.store.CountUsers(r.Context()); err == nil && n == 1 {
		_ = s.store.UpdateAccount(r.Context(), acct.User.ID, acct.Email, model.RoleAdmin, model.StatusActive)
		acct, _ = s.store.AccountByID(r.Context(), acct.User.ID)
	}

	if err := s.startSession(r.Context(), w, r, acct.User.ID); err != nil {
		s.log.Error("start session", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "account created but sign-in failed")
		return
	}
	if req.Email != "" && s.settingBool(r.Context(), settingRequireVerify, false) {
		s.sendVerificationMail(r.Context(), acct)
	}
	_ = s.store.Log(r.Context(), acct.User.ID, "user.register", "user", acct.User.ID, "")
	writeJSON(w, http.StatusCreated, acct.Self())
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, "login", 20, 8) {
		return
	}
	var req loginRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	login := strings.TrimSpace(req.Login)
	if login == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "enter your username and password")
		return
	}

	acct, err := s.store.AccountByUsername(r.Context(), login)
	if err != nil {
		acct, err = s.store.AccountByEmail(r.Context(), strings.ToLower(login))
	}
	if err != nil {
		if err != store.ErrNotFound {
			writeStoreError(s, w, r, err, "account not found")
			return
		}
		s.equaliseTiming(req.Password)
		writeError(w, http.StatusUnauthorized, "bad_credentials", "wrong username or password")
		return
	}

	if acct.PasswordHash == "" {
		// Account exists but has no password: it is an OAuth-only account.
		s.equaliseTiming(req.Password)
		writeError(w, http.StatusUnauthorized, "no_password", "this account uses a social login or a magic link")
		return
	}

	ok, err := auth.VerifyPassword(acct.PasswordHash, req.Password)
	if err != nil {
		s.log.Error("verify password", "err", err, "user", acct.User.ID)
		writeError(w, http.StatusInternalServerError, "internal", "could not verify the password")
		return
	}
	if !ok {
		writeError(w, http.StatusUnauthorized, "bad_credentials", "wrong username or password")
		return
	}
	if acct.User.Status == model.StatusBanned {
		writeError(w, http.StatusForbidden, "banned", "this account has been suspended")
		return
	}
	if s.settingBool(r.Context(), settingRequireVerify, false) && acct.Email != "" && !acct.EmailVerified {
		writeError(w, http.StatusForbidden, "unverified", "confirm your e-mail address before signing in")
		return
	}

	if err := s.startSession(r.Context(), w, r, acct.User.ID); err != nil {
		s.log.Error("start session", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not sign you in")
		return
	}
	writeJSON(w, http.StatusOK, acct.Self())
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if hash := currentSessionHash(r); hash != "" {
		if err := s.store.DeleteSession(r.Context(), hash); err != nil {
			s.log.Error("delete session", "err", err)
		}
	}
	s.clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not signed in")
		return
	}
	acct, err := s.store.AccountByID(r.Context(), u.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, acct.Self())
}

func (s *Server) handleSignOutOthers(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not signed in")
		return
	}
	if err := s.store.DeleteOtherSessions(r.Context(), u.ID, currentSessionHash(r)); err != nil {
		s.log.Error("delete other sessions", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not sign out other sessions")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- magic links ----

type emailRequest struct {
	Email string `json:"email"`
}

func (s *Server) handleMagicRequest(w http.ResponseWriter, r *http.Request) {
	// Tighter limit than log-in: each request sends a mail.
	if !s.allow(w, r, "magic", 5, 3) {
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
			"this instance has no mail server configured, so magic links are unavailable")
		return
	}

	acct, err := s.store.AccountByEmail(r.Context(), email)
	if err != nil && err != store.ErrNotFound {
		writeStoreError(s, w, r, err, "account not found")
		return
	}

	if err == nil && acct.User.Status == model.StatusActive {
		raw, hash, tokenErr := auth.NewToken()
		if tokenErr != nil {
			s.log.Error("create magic token", "err", tokenErr)
		} else {
			expires := time.Now().Add(30 * time.Minute)
			if storeErr := s.store.CreateLoginToken(r.Context(), hash, email, &acct.User.ID, "magic", expires.Unix()); storeErr != nil {
				s.log.Error("store magic token", "err", storeErr)
			} else {
				link := s.cfg.PublicURL + "/auth/magic?token=" + raw
				body := "Use this link to sign in to " + s.getSetting(r.Context(), settingSiteName, "knot") + ":\n\n" +
					link + "\n\nThe link expires in 30 minutes and can be used once.\n"
				s.queueMailTo(r.Context(), email, "Sign in to "+s.getSetting(r.Context(), settingSiteName, "knot"), body)
			}
		}
	}

	// Always the same answer: whether an address has an account is not public.
	writeJSON(w, http.StatusAccepted, map[string]string{
		"message": "if that address belongs to an account, a sign-in link is on its way",
	})
}

type tokenRequest struct {
	Token string `json:"token"`
}

func (s *Server) handleMagicConsume(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, "magic-consume", 20, 10) {
		return
	}
	var req tokenRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "missing token")
		return
	}

	userID, _, err := s.store.ConsumeLoginToken(r.Context(), auth.HashToken(req.Token), "magic")
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

	if err := s.startSession(r.Context(), w, r, acct.User.ID); err != nil {
		s.log.Error("start session", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not sign you in")
		return
	}
	writeJSON(w, http.StatusOK, acct.Self())
}

// handleVerifyEmail consumes an e-mail verification token.
func (s *Server) handleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if !s.allow(w, r, "verify", 20, 10) {
		return
	}
	var req tokenRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	userID, _, err := s.store.ConsumeLoginToken(r.Context(), auth.HashToken(req.Token), "verify")
	if err != nil {
		writeStoreError(s, w, r, err, "that link is not valid, or has already been used")
		return
	}
	if !userID.Valid {
		writeError(w, http.StatusBadRequest, "bad_token", "that link is not valid")
		return
	}
	if err := s.store.SetEmailVerified(r.Context(), userID.Int64, true); err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// sendVerificationMail issues a verification token and mails it.
func (s *Server) sendVerificationMail(ctx context.Context, acct *store.Account) {
	if acct.Email == "" || !s.smtpConfig(ctx).Enabled() {
		return
	}
	raw, hash, err := auth.NewToken()
	if err != nil {
		s.log.Error("create verify token", "err", err)
		return
	}
	expires := time.Now().Add(48 * time.Hour)
	if err := s.store.CreateLoginToken(ctx, hash, acct.Email, &acct.User.ID, "verify", expires.Unix()); err != nil {
		s.log.Error("store verify token", "err", err)
		return
	}
	site := s.getSetting(ctx, settingSiteName, "knot")
	link := s.cfg.PublicURL + "/auth/verify?token=" + raw
	body := "Confirm your e-mail address for " + site + ":\n\n" + link + "\n\nThe link expires in 48 hours.\n"
	s.queueMailTo(ctx, acct.Email, "Confirm your e-mail for "+site, body)
}

// ---- own account ----

type profileRequest struct {
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url"`
}

func (s *Server) handleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not signed in")
		return
	}
	if !s.allow(w, r, "profile", 20, 10) {
		return
	}
	var req profileRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	displayName := strings.TrimSpace(req.DisplayName)
	bio := strings.TrimSpace(req.Bio)
	avatar := strings.TrimSpace(req.AvatarURL)

	if len([]rune(displayName)) > 60 {
		writeError(w, http.StatusBadRequest, "invalid", "display name is too long")
		return
	}
	if len([]rune(bio)) > 600 {
		writeError(w, http.StatusBadRequest, "invalid", "bio is too long")
		return
	}
	if avatar != "" && !strings.HasPrefix(avatar, "https://") && !strings.HasPrefix(avatar, "/uploads/") {
		writeError(w, http.StatusBadRequest, "invalid", "avatar must be an https:// URL or an uploaded image")
		return
	}

	if err := s.store.UpdateProfile(r.Context(), u.ID, displayName, bio, avatar); err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	acct, err := s.store.AccountByID(r.Context(), u.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	writeJSON(w, http.StatusOK, acct.Self())
}

type passwordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not signed in")
		return
	}
	if !s.allow(w, r, "password", 10, 5) {
		return
	}
	var req passwordRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	acct, err := s.store.AccountByID(r.Context(), u.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	if acct.PasswordHash != "" {
		ok, err := auth.VerifyPassword(acct.PasswordHash, req.CurrentPassword)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not verify the current password")
			return
		}
		if !ok {
			writeError(w, http.StatusForbidden, "bad_credentials", "current password is wrong")
			return
		}
	}
	if err := auth.ValidatePassword(req.NewPassword); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_password", err.Error())
		return
	}

	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store the password")
		return
	}
	if err := s.store.SetPassword(r.Context(), u.ID, hash); err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	// Changing a password signs out every other device.
	_ = s.store.DeleteOtherSessions(r.Context(), u.ID, currentSessionHash(r))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleChangeEmail(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "not signed in")
		return
	}
	if !s.allow(w, r, "email", 10, 5) {
		return
	}
	var req emailRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email != "" {
		if err := auth.ValidateEmail(email); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
			return
		}
		if s.spamRules(r.Context()).EmailBlocked(email) {
			writeError(w, http.StatusBadRequest, "email_blocked",
				"that e-mail provider is not accepted here. Please use a different address.")
			return
		}
	}
	if err := s.store.SetEmail(r.Context(), u.ID, email); err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	acct, err := s.store.AccountByID(r.Context(), u.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "account not found")
		return
	}
	s.sendVerificationMail(r.Context(), acct)
	writeJSON(w, http.StatusOK, acct.Self())
}
