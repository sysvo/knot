package api

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/sysvo/knot/internal/model"
	"github.com/sysvo/knot/internal/store"
)

// mentionRE finds @username references in Markdown.
var mentionRE = regexp.MustCompile(`@([a-zA-Z0-9][a-zA-Z0-9_-]{2,31})`)

// canEdit reports whether u may modify content authored by authorID.
func canEdit(u *model.User, authorID *int64) bool {
	if u == nil || authorID == nil {
		return false
	}
	return u.CanModerate() || *authorID == u.ID
}

// ---- boards ----

func (s *Server) handleListBoards(w http.ResponseWriter, r *http.Request) {
	boards, err := s.store.ListBoards(r.Context())
	if err != nil {
		writeStoreError(s, w, r, err, "boards not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"boards": boards})
}

func (s *Server) handleGetBoard(w http.ResponseWriter, r *http.Request) {
	board, err := s.store.BoardBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		writeStoreError(s, w, r, err, "no such board")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"board": board})
}

func (s *Server) handleListBoardThreads(w http.ResponseWriter, r *http.Request) {
	board, err := s.store.BoardBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		writeStoreError(s, w, r, err, "no such board")
		return
	}
	page, perPage, offset := pageParams(r, 20, 100)
	sort := store.ThreadSort(r.URL.Query().Get("sort"))

	threads, total, err := s.store.ListThreads(r.Context(), store.ThreadQuery{
		BoardID: &board.ID, Sort: sort, Limit: perPage, Offset: offset,
	})
	if err != nil {
		writeStoreError(s, w, r, err, "threads not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"board":   board,
		"threads": threads,
		"page":    model.Page{Number: page, PerPage: perPage, Total: total, HasNext: offset+len(threads) < int(total)},
	})
}

// ---- threads ----

func (s *Server) handleListThreads(w http.ResponseWriter, r *http.Request) {
	page, perPage, offset := pageParams(r, 20, 100)
	sort := store.ThreadSort(r.URL.Query().Get("sort"))

	threads, total, err := s.store.ListThreads(r.Context(), store.ThreadQuery{
		Sort: sort, Limit: perPage, Offset: offset,
	})
	if err != nil {
		writeStoreError(s, w, r, err, "threads not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"threads": threads,
		"page":    model.Page{Number: page, PerPage: perPage, Total: total, HasNext: offset+len(threads) < int(total)},
	})
}

func (s *Server) handleGetThread(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid thread id")
		return
	}

	thread, err := s.store.ThreadByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such thread")
		return
	}

	viewer := currentUser(r)
	isAuthor := viewer != nil && thread.Author != nil && viewer.ID == thread.Author.ID
	// Held content is visible to its author and to moderators, and to nobody
	// else; a 404 rather than a 403 avoids confirming that it exists.
	if thread.Deleted && !viewer.CanModerate() {
		writeError(w, http.StatusNotFound, "not_found", "no such thread")
		return
	}
	if thread.Pending && !viewer.CanModerate() && !isAuthor {
		writeError(w, http.StatusNotFound, "not_found", "no such thread")
		return
	}

	var viewerID int64
	if viewer != nil {
		viewerID = viewer.ID
	}

	page, perPage, offset := pageParams(r, 20, 100)
	posts, total, err := s.store.ListPosts(r.Context(), id, viewerID, perPage, offset, viewer.CanModerate() || isAuthor)
	if err != nil {
		writeStoreError(s, w, r, err, "posts not found")
		return
	}

	// Only count a read once the thread actually rendered, and never for
	// something that is still waiting for review.
	if !thread.Pending {
		_ = s.store.IncrementThreadViews(r.Context(), id)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"thread": thread,
		"posts":  posts,
		"page":   model.Page{Number: page, PerPage: perPage, Total: total, HasNext: offset+len(posts) < int(total)},
	})
}

func (s *Server) handleGetPost(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid post id")
		return
	}
	viewer := currentUser(r)
	var viewerID int64
	if viewer != nil {
		viewerID = viewer.ID
	}
	post, err := s.store.PostByID(r.Context(), id, viewerID)
	if err != nil {
		writeStoreError(s, w, r, err, "no such post")
		return
	}

	// This endpoint mirrors the thread view: a held or removed post, and any post
	// in a held or removed thread, is only visible to its author and moderators.
	// Without the thread check a single approved reply would expose a thread that
	// moderators are still holding.
	thread, err := s.store.ThreadByID(r.Context(), post.ThreadID)
	if err != nil {
		writeStoreError(s, w, r, err, "no such post")
		return
	}
	moderator := viewer.CanModerate()
	isPostAuthor := viewer != nil && post.Author != nil && viewer.ID == post.Author.ID
	isThreadAuthor := viewer != nil && thread.Author != nil && viewer.ID == thread.Author.ID

	if !moderator && ((post.Deleted && !isPostAuthor) || (post.Pending && !isPostAuthor)) {
		writeError(w, http.StatusNotFound, "not_found", "no such post")
		return
	}
	if !moderator && !isThreadAuthor && (thread.Deleted || thread.Pending) {
		writeError(w, http.StatusNotFound, "not_found", "no such post")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"post": post})
}

type threadRequest struct {
	Title   string `json:"title"`
	Body    string `json:"body"`
	BoardID int64  `json:"board_id"`
}

const (
	maxTitleLength = 140
	maxBodyLength  = 20000
)

func (s *Server) handleCreateThread(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in to start a thread")
		return
	}
	if !s.allow(w, r, "thread", 10, 5) {
		return
	}

	board, err := s.store.BoardBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		writeStoreError(s, w, r, err, "no such board")
		return
	}

	var req threadRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	title, body, ok := s.validateContent(w, req.Title, req.Body)
	if !ok {
		return
	}

	verdict, err := s.assess(r.Context(), u)
	if err != nil {
		writeStoreError(s, w, r, err, "could not check your posting allowance")
		return
	}
	if !s.checkQuota(w, r, u, verdict) {
		return
	}

	html := s.md.Render(body)
	if !s.checkLinks(w, r, html, verdict) {
		return
	}
	excerpt := s.md.Excerpt(body)

	thread, err := s.store.CreateThread(r.Context(), board.ID, u.ID, title, excerpt, body, html, s.md.PlainText(body), !verdict.NeedsApproval)
	if err != nil {
		writeStoreError(s, w, r, err, "could not create the thread")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"thread": thread})
}

func (s *Server) handleUpdateThread(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid thread id")
		return
	}
	thread, err := s.store.ThreadByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such thread")
		return
	}
	if !canEdit(u, authorID(thread.Author)) {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot edit this thread")
		return
	}

	var req threadRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	title := strings.TrimSpace(req.Title)
	if n := len([]rune(title)); n < 3 || n > maxTitleLength {
		writeError(w, http.StatusBadRequest, "invalid_title", "title must be between 3 and 140 characters")
		return
	}
	if err := s.store.UpdateThreadTitle(r.Context(), id, title); err != nil {
		writeStoreError(s, w, r, err, "could not update the thread")
		return
	}
	_ = s.store.Log(r.Context(), u.ID, "thread.retitle", "thread", id, title)

	updated, err := s.store.ThreadByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such thread")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": updated})
}

func (s *Server) handleDeleteThread(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid thread id")
		return
	}
	thread, err := s.store.ThreadByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such thread")
		return
	}
	if !canEdit(u, authorID(thread.Author)) {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot delete this thread")
		return
	}
	if err := s.store.SoftDeleteThread(r.Context(), id, u.ID); err != nil {
		writeStoreError(s, w, r, err, "could not delete the thread")
		return
	}
	_ = s.store.Log(r.Context(), u.ID, "thread.delete", "thread", id, thread.Title)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTogglePin(w http.ResponseWriter, r *http.Request) {
	if s.mustModerator(w, r) == nil {
		return
	}
	s.toggleThreadFlag(w, r, "pin")
}

func (s *Server) handleToggleLock(w http.ResponseWriter, r *http.Request) {
	if s.mustModerator(w, r) == nil {
		return
	}
	s.toggleThreadFlag(w, r, "lock")
}

func (s *Server) toggleThreadFlag(w http.ResponseWriter, r *http.Request, flag string) {
	u := currentUser(r)
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid thread id")
		return
	}
	thread, err := s.store.ThreadByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such thread")
		return
	}

	pinned, locked := thread.Pinned, thread.Locked
	if flag == "pin" {
		pinned = !pinned
	} else {
		locked = !locked
	}
	if err := s.store.SetThreadFlags(r.Context(), id, pinned, locked); err != nil {
		writeStoreError(s, w, r, err, "could not update the thread")
		return
	}
	_ = s.store.Log(r.Context(), u.ID, "thread."+flag, "thread", id, "")

	updated, err := s.store.ThreadByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such thread")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"thread": updated})
}

// ---- posts ----

type postRequest struct {
	Body string `json:"body"`
}

func (s *Server) handleCreatePost(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in to reply")
		return
	}
	if !s.allow(w, r, "post", 20, 8) {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid thread id")
		return
	}
	thread, err := s.store.ThreadByID(r.Context(), id)
	if err != nil {
		writeStoreError(s, w, r, err, "no such thread")
		return
	}
	if thread.Deleted {
		writeError(w, http.StatusNotFound, "not_found", "no such thread")
		return
	}
	// A thread waiting for review is visible to its author and moderators; nobody
	// else may add to it, or the queue would be interactive.
	if thread.Pending && !u.CanModerate() && (thread.Author == nil || thread.Author.ID != u.ID) {
		writeError(w, http.StatusNotFound, "not_found", "no such thread")
		return
	}
	if thread.Locked && !u.CanModerate() {
		writeError(w, http.StatusForbidden, "locked", "this thread is locked")
		return
	}

	var req postRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	_, body, ok := s.validateContent(w, "x", req.Body)
	if !ok {
		return
	}

	verdict, err := s.assess(r.Context(), u)
	if err != nil {
		writeStoreError(s, w, r, err, "could not check your posting allowance")
		return
	}
	if !s.checkQuota(w, r, u, verdict) {
		return
	}

	html := s.md.Render(body)
	if !s.checkLinks(w, r, html, verdict) {
		return
	}

	post, err := s.store.CreatePost(r.Context(), id, u.ID, body, html, s.md.PlainText(body), !verdict.NeedsApproval)
	if err != nil {
		writeStoreError(s, w, r, err, "could not post the reply")
		return
	}

	// A held post is invisible to everyone else, so nothing is announced until a
	// moderator approves it.
	if !post.Pending {
		s.notifyPostPublished(r.Context(), u, post)
	}

	writeJSON(w, http.StatusCreated, map[string]any{"post": post})
}

// notifyMentions resolves @username references to accounts and notifies them.
func (s *Server) notifyMentions(ctx context.Context, actor *model.User, body string, threadID, postID int64) {
	seen := map[string]bool{}
	matches := mentionRE.FindAllStringSubmatch(body, 10)
	for _, m := range matches {
		name := m[1]
		lower := strings.ToLower(name)
		if seen[lower] {
			continue
		}
		seen[lower] = true

		acct, err := s.store.AccountByUsername(ctx, name)
		if err != nil {
			continue // unknown handle: silently ignored
		}
		if acct.User.Status != model.StatusActive {
			continue
		}
		_ = s.store.Notify(ctx, acct.User.ID, actor.ID, "mention", threadID, postID)
	}
}

func (s *Server) handleUpdatePost(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid post id")
		return
	}
	post, err := s.store.PostByID(r.Context(), id, u.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "no such post")
		return
	}
	if post.Deleted {
		writeError(w, http.StatusNotFound, "not_found", "no such post")
		return
	}
	if !canEdit(u, authorID(post.Author)) {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot edit this post")
		return
	}

	var req postRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	_, body, ok := s.validateContent(w, "x", req.Body)
	if !ok {
		return
	}

	// Re-check links on edit: otherwise a new account could post clean text and
	// then edit the link in.
	verdict, err := s.assess(r.Context(), u)
	if err != nil {
		writeStoreError(s, w, r, err, "could not check your posting allowance")
		return
	}
	if !s.checkLinks(w, r, s.md.Render(body), verdict) {
		return
	}

	if err := s.store.UpdatePost(r.Context(), id, body, s.md.Render(body), s.md.PlainText(body)); err != nil {
		writeStoreError(s, w, r, err, "could not update the post")
		return
	}

	updated, err := s.store.PostByID(r.Context(), id, u.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "no such post")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"post": updated})
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid post id")
		return
	}
	post, err := s.store.PostByID(r.Context(), id, u.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "no such post")
		return
	}
	if !canEdit(u, authorID(post.Author)) {
		writeError(w, http.StatusForbidden, "forbidden", "you cannot delete this post")
		return
	}
	if err := s.store.SoftDeletePost(r.Context(), id, u.ID); err != nil {
		writeStoreError(s, w, r, err, "could not delete the post")
		return
	}
	_ = s.store.Log(r.Context(), u.ID, "post.delete", "post", id, "")
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

type voteRequest struct {
	Value int `json:"value"`
}

func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in to vote")
		return
	}
	if !s.allow(w, r, "vote", 60, 30) {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid post id")
		return
	}
	var req voteRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Value < -1 || req.Value > 1 {
		writeError(w, http.StatusBadRequest, "invalid", "vote must be 1, -1 or 0")
		return
	}

	score, myVote, err := s.store.VotePost(r.Context(), id, u.ID, req.Value)
	if err != nil {
		writeStoreError(s, w, r, err, "no such post")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"score": score, "my_vote": myVote})
}

type reportRequest struct {
	Reason  string `json:"reason"`
	Details string `json:"details"`
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in to report a post")
		return
	}
	if !s.allow(w, r, "report", 10, 5) {
		return
	}
	id, ok := idParam(r, "id")
	if !ok {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid post id")
		return
	}
	post, err := s.store.PostByID(r.Context(), id, u.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "no such post")
		return
	}

	var req reportRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		writeError(w, http.StatusBadRequest, "invalid", "choose a reason for the report")
		return
	}
	if len(reason) > 60 {
		reason = reason[:60]
	}
	details := strings.TrimSpace(req.Details)
	if len([]rune(details)) > 1000 {
		writeError(w, http.StatusBadRequest, "invalid", "report details are too long")
		return
	}

	reportID, err := s.store.CreateReport(r.Context(), u.ID, &id, &post.ThreadID, reason, details)
	if err != nil {
		writeStoreError(s, w, r, err, "could not file the report")
		return
	}
	_ = s.store.Log(r.Context(), u.ID, "report.create", "post", id, reason)
	writeJSON(w, http.StatusCreated, map[string]any{"id": reportID})
}

// ---- search and profiles ----

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	board := strings.TrimSpace(r.URL.Query().Get("board"))
	limit := intQuery(r, "limit", 30)
	if limit < 1 || limit > 100 {
		limit = 30
	}
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"query": "", "results": []model.SearchHit{}})
		return
	}

	hits, err := s.store.Search(r.Context(), q, board, limit)
	if err != nil {
		s.log.Error("search", "err", err, "q", q)
		writeError(w, http.StatusInternalServerError, "internal", "search failed")
		return
	}
	// The store returns the snippet with match markers; escaping happens here, once,
	// so no caller can accidentally hand raw post text to the browser as HTML.
	for i := range hits {
		hits[i].Snippet = snippetHTML(hits[i].Snippet)
	}
	writeJSON(w, http.StatusOK, map[string]any{"query": q, "results": hits})
}

func (s *Server) handleGetUser(w http.ResponseWriter, r *http.Request) {
	acct, err := s.store.AccountByUsername(r.Context(), r.PathValue("username"))
	if err != nil {
		writeStoreError(s, w, r, err, "no such user")
		return
	}
	posts, err := s.store.UserPostCount(r.Context(), acct.User.ID)
	if err != nil {
		writeStoreError(s, w, r, err, "no such user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"user":       acct.User,
		"post_count": posts,
	})
}

func (s *Server) handleUserPosts(w http.ResponseWriter, r *http.Request) {
	acct, err := s.store.AccountByUsername(r.Context(), r.PathValue("username"))
	if err != nil {
		writeStoreError(s, w, r, err, "no such user")
		return
	}
	limit := intQuery(r, "limit", 20)
	if limit < 1 || limit > 50 {
		limit = 20
	}
	posts, err := s.store.RecentPostsByUser(r.Context(), acct.User.ID, limit)
	if err != nil {
		writeStoreError(s, w, r, err, "no such user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": posts})
}

// ---- notifications ----

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return
	}
	limit := intQuery(r, "limit", 40)
	if limit < 1 || limit > 100 {
		limit = 40
	}
	items, unread, err := s.store.ListNotifications(r.Context(), u.ID, limit)
	if err != nil {
		writeStoreError(s, w, r, err, "could not load notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": items, "unread": unread})
}

func (s *Server) handleReadNotifications(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return
	}
	if err := s.store.MarkNotificationsRead(r.Context(), u.ID, false); err != nil {
		writeStoreError(s, w, r, err, "could not update notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleDeleteNotifications(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return
	}
	if err := s.store.MarkNotificationsRead(r.Context(), u.ID, true); err != nil {
		writeStoreError(s, w, r, err, "could not clear notifications")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---- helpers ----

// authorID safely converts an embedded author into a pointer.
func authorID(u *model.User) *int64 {
	if u == nil {
		return nil
	}
	id := u.ID
	return &id
}

// validateContent normalises and checks a title/body pair. It writes the error
// response itself and reports whether the caller may continue.
func (s *Server) validateContent(w http.ResponseWriter, title, body string) (string, string, bool) {
	title = strings.TrimSpace(title)
	body = strings.TrimSpace(body)

	if title != "x" { // "x" is the sentinel used for post-only requests
		if n := len([]rune(title)); n < 3 || n > maxTitleLength {
			writeError(w, http.StatusBadRequest, "invalid_title", "title must be between 3 and 140 characters")
			return "", "", false
		}
	}
	if body == "" {
		writeError(w, http.StatusBadRequest, "empty_post", "the post is empty")
		return "", "", false
	}
	if len([]rune(body)) > maxBodyLength {
		writeError(w, http.StatusBadRequest, "too_long", "the post is too long")
		return "", "", false
	}
	return title, body, true
}

// mustModerator writes a 403 and returns nil unless the caller moderates.
func (s *Server) mustModerator(w http.ResponseWriter, r *http.Request) *model.User {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in first")
		return nil
	}
	if !u.CanModerate() {
		writeError(w, http.StatusForbidden, "forbidden", "moderators only")
		return nil
	}
	return u
}
