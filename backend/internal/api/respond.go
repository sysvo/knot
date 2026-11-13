package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/sysvo/knot/internal/store"
)

// errorBody is the shape of every error response.
type errorBody struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// writeJSON writes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode response", "err", err)
	}
}

// writeError writes a structured error.
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorDetail{Code: code, Message: message}})
}

// writeStoreError maps a store error onto the right response.
func writeStoreError(s *Server, w http.ResponseWriter, r *http.Request, err error, notFoundMessage string) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "not_found", notFoundMessage)
	case errors.Is(err, store.ErrConflict):
		writeError(w, http.StatusConflict, "conflict", "that value is already taken")
	default:
		s.log.Error("request failed", "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "something went wrong")
	}
}

// decodeJSON reads a JSON request body into dst, rejecting unknown fields so
// typos in the client surface immediately instead of being silently ignored.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	ct := r.Header.Get("Content-Type")
	if ct != "" && !isJSONContentType(ct) {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "expected application/json")
		return false
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read request body")
		return false
	}
	return true
}

func isJSONContentType(ct string) bool {
	for i := 0; i < len(ct); i++ {
		if ct[i] == ';' {
			ct = ct[:i]
			break
		}
	}
	// Only application/json: text/plain is exactly what a cross-site form or a
	// no-cors fetch can send, and accepting it would remove one layer of CSRF
	// defence in depth for the sake of a client that does not exist here.
	return ct == "application/json"
}

// pageParams reads ?page= and ?per_page= and clamps them.
func pageParams(r *http.Request, defaultPerPage, maxPerPage int) (page, perPage, offset int) {
	page = intQuery(r, "page", 1)
	if page < 1 {
		page = 1
	}
	perPage = intQuery(r, "per_page", defaultPerPage)
	if perPage < 1 {
		perPage = defaultPerPage
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}
	return page, perPage, (page - 1) * perPage
}

// intQuery parses a query parameter as an integer, falling back to def.
func intQuery(r *http.Request, name string, def int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return def
	}
	var n int
	neg := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '-' && i == 0 {
			neg = true
			continue
		}
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if neg {
		n = -n
	}
	return n
}

// idParam reads a numeric path parameter.
func idParam(r *http.Request, name string) (int64, bool) {
	raw := r.PathValue(name)
	if raw == "" {
		return 0, false
	}
	var n int64
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
		if n > 1<<53 {
			return 0, false
		}
	}
	if n == 0 {
		return 0, false
	}
	return n, true
}
