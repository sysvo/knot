package api

import (
	"net/http"
	"net/http/pprof"
)

// adminOnly wraps a handler so that only administrators reach it.
func (s *Server) adminOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.mustAdmin(w, r) == nil {
			return
		}
		next.ServeHTTP(w, r)
	})
}

// pprofHandler serves the standard Go profiles to administrators.
//
// The profiles are what turn "the page is slow" into a number: a CPU profile of a
// busy instance, or a heap profile after a memory spike, names the function instead
// of leaving it to guesswork. They are mounted on their own mux rather than the
// default one, and behind the admin role, because a profile is both expensive to
// collect and revealing about internal traffic.
func (s *Server) pprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/admin/pprof/", pprof.Index)
	mux.HandleFunc("/api/v1/admin/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/api/v1/admin/pprof/profile", pprof.Profile)
	mux.HandleFunc("/api/v1/admin/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/api/v1/admin/pprof/trace", pprof.Trace)
	return mux
}
