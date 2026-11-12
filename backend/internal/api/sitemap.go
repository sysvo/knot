package api

import (
	"net/http"
)

// handleSitemap lists the public URLs a crawler should know about.
//
// It exists so the frontend's sitemap can be generated from one request instead
// of paging through every board.
func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	threads, boards, err := s.store.SitemapEntries(r.Context(), 5000)
	if err != nil {
		writeStoreError(s, w, r, err, "could not build the sitemap")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"threads": threads, "boards": boards})
}
