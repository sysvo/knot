package api

import (
	"net/http"
	"strings"

	"github.com/sysvo/knot/internal/auth"
	"github.com/sysvo/knot/internal/model"
)

// mustAdmin writes a 403 and returns nil unless the caller is an administrator.
func (s *Server) mustAdmin(w http.ResponseWriter, r *http.Request) *model.User {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return nil
	}
	if !u.IsAdmin() {
		writeError(w, http.StatusForbidden, "forbidden", "administrators only")
		return nil
	}
	return u
}

func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	if s.mustAdmin(w, r) == nil {
		return
	}
	stats, err := s.store.AdminStats(r.Context())
	if err != nil {
		writeStoreError(s, w, r, err, "no stats")
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (s *Server) handleAdminUsers(w http.ResponseWriter, r *http.Request) {
	if s.mustAdmin(w, r) == nil {
		return
	}
	page, perPage, offset := pageParams(r, 50, 200)
	users, total, err := s.store.ListUsers(r.Context(), strings.TrimSpace(r.URL.Query().Get("q")), perPage, offset)
	if err != nil {
		writeStoreError(s, w, r, err, "no users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"users": users,
		"page":  model.Page{Number: page, PerPage: perPage, Total: total, HasNext: offset+len(users) < int(total)},
	})
}

type adminUserRequest struct {
	Role   string `json:"role"`
	Status string `json:"status"`
	Email  string `json:"email"`
}

func (s *Server) handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid user id")
		return
	}
	acct, err := s.store.AccountByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such user")
		return
	}

	var req adminUserRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	role := req.Role
	if role == "" {
		role = acct.User.Role
	}
	status := req.Status
	if status == "" {
		status = acct.User.Status
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if email == "" {
		email = acct.Email
	}
	if email != "" {
		if err := auth.ValidateEmail(email); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_email", err.Error())
			return
		}
	}

	switch role {
	case model.RoleMember, model.RoleModerator, model.RoleAdmin:
	default:
		writeError(w, http.StatusBadRequest, "invalid_role", "unknown role")
		return
	}
	switch status {
	case model.StatusActive, model.StatusBanned:
	default:
		writeError(w, http.StatusBadRequest, "invalid_status", "unknown status")
		return
	}

	// Refuse to remove the last administrator: it is the easiest way to lock
	// yourself out of a self-hosted instance permanently.
	if acct.User.Role == model.RoleAdmin && role != model.RoleAdmin {
		admins, err := s.store.CountAdmins(r.Context())
		if err != nil {
			writeStoreError(s, w, r, err, "no such user")
			return
		}
		if admins <= 1 {
			writeError(w, http.StatusBadRequest, "last_admin", "there must always be at least one administrator")
			return
		}
	}

	if err := s.store.UpdateAccount(r.Context(), id, email, role, status); err != nil {
		writeStoreError(s, w, r, err, "no such user")
		return
	}
	if status == model.StatusBanned {
		// Drop every session so a ban takes effect immediately.
		_ = s.store.DeleteOtherSessions(r.Context(), id, "")
	}
	_ = s.store.Log(r.Context(), admin.ID, "admin.user.update", "user", id,
		"role="+role+" status="+status)

	updated, err := s.store.AccountByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such user")
		return
	}
	writeJSON(w, http.StatusOK, updated.Self())
}

func (s *Server) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	if s.mustModerator(w, r) == nil {
		return
	}
	onlyOpen := r.URL.Query().Get("status") != "all"
	page, perPage, offset := pageParams(r, 50, 200)
	reports, total, err := s.store.ListReports(r.Context(), onlyOpen, perPage, offset)
	if err != nil {
		writeStoreError(s, w, r, err, "no reports")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"reports": reports,
		"page":    model.Page{Number: page, PerPage: perPage, Total: total, HasNext: offset+len(reports) < int(total)},
	})
}

type resolveRequest struct {
	Status string `json:"status"`
}

func (s *Server) handleAdminResolveReport(w http.ResponseWriter, r *http.Request) {
	mod := s.mustModerator(w, r)
	if mod == nil {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid report id")
		return
	}
	var req resolveRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	status := req.Status
	if status == "" {
		status = "resolved"
	}
	if status != "resolved" && status != "dismissed" {
		writeError(w, http.StatusBadRequest, "invalid", "status must be resolved or dismissed")
		return
	}
	if err := s.store.ResolveReport(r.Context(), id, mod.ID, status); err != nil {
		writeStoreError(s, w, r, err, "no such report")
		return
	}
	_ = s.store.Log(r.Context(), mod.ID, "report."+status, "report", id, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAdminGetSettings(w http.ResponseWriter, r *http.Request) {
	if s.mustAdmin(w, r) == nil {
		return
	}
	all, err := s.store.Settings(r.Context())
	if err != nil {
		writeStoreError(s, w, r, err, "no settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": all})
}

// editableSettings is the whitelist of keys the admin API may write.
var editableSettings = map[string]bool{
	settingSiteName:         true,
	settingSiteDescription:  true,
	settingAllowRegister:    true,
	settingRequireVerify:    true,
	settingRegistrationNote: true,
}

func (s *Server) handleAdminSetSettings(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	var req struct {
		Settings map[string]string `json:"settings"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}

	clean := map[string]string{}
	for k, v := range req.Settings {
		if !editableSettings[k] {
			continue // ignore unknown keys rather than storing junk
		}
		clean[k] = strings.TrimSpace(v)
	}
	if name, ok := clean[settingSiteName]; ok && (name == "" || len([]rune(name)) > 40) {
		writeError(w, http.StatusBadRequest, "invalid", "site name must be 1-40 characters")
		return
	}
	if err := s.store.SetSettings(r.Context(), clean); err != nil {
		writeStoreError(s, w, r, err, "could not save settings")
		return
	}
	_ = s.store.Log(r.Context(), admin.ID, "admin.settings", "settings", 0, "")

	all, err := s.store.Settings(r.Context())
	if err != nil {
		writeStoreError(s, w, r, err, "no settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"settings": all})
}

type boardRequest struct {
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Position    int    `json:"position"`
}

func (s *Server) handleAdminCreateBoard(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	var req boardRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	slug, name, description, ok := s.validateBoard(w, req)
	if !ok {
		return
	}

	board, err := s.store.CreateBoard(r.Context(), slug, name, description, req.Position)
	if err != nil {
		writeStoreError(s, w, r, err, "could not create the board")
		return
	}
	_ = s.store.Log(r.Context(), admin.ID, "admin.board.create", "board", board.ID, slug)
	writeJSON(w, http.StatusCreated, map[string]any{"board": board})
}

func (s *Server) handleAdminUpdateBoard(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid board id")
		return
	}
	var req boardRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	slug, name, description, ok := s.validateBoard(w, req)
	if !ok {
		return
	}
	if err := s.store.UpdateBoard(r.Context(), id, slug, name, description, req.Position); err != nil {
		writeStoreError(s, w, r, err, "could not update the board")
		return
	}
	_ = s.store.Log(r.Context(), admin.ID, "admin.board.update", "board", id, slug)

	board, err := s.store.BoardByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such board")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"board": board})
}

func (s *Server) handleAdminDeleteBoard(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid board id")
		return
	}
	if err := s.store.DeleteBoard(r.Context(), id); err != nil {
		writeStoreError(s, w, r, err, "could not delete the board")
		return
	}
	_ = s.store.Log(r.Context(), admin.ID, "admin.board.delete", "board", id, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) validateBoard(w http.ResponseWriter, req boardRequest) (slug, name, description string, ok bool) {
	slug = strings.ToLower(strings.TrimSpace(req.Slug))
	name = strings.TrimSpace(req.Name)
	description = strings.TrimSpace(req.Description)

	if err := auth.ValidateSlug(slug); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_slug", err.Error())
		return "", "", "", false
	}
	if n := len([]rune(name)); n < 2 || n > 60 {
		writeError(w, http.StatusBadRequest, "invalid_name", "board name must be 2-60 characters")
		return "", "", "", false
	}
	if len([]rune(description)) > 300 {
		writeError(w, http.StatusBadRequest, "invalid_description", "description is too long")
		return "", "", "", false
	}
	return slug, name, description, true
}

func (s *Server) handleAdminAudit(w http.ResponseWriter, r *http.Request) {
	if s.mustModerator(w, r) == nil {
		return
	}
	limit := intQuery(r, "limit", 100)
	if limit < 1 || limit > 500 {
		limit = 100
	}
	entries, err := s.store.ListAudit(r.Context(), limit)
	if err != nil {
		writeStoreError(s, w, r, err, "no audit entries")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}
