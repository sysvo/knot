package api

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/sysvo/knot/internal/backup"
)

// backupNameRE is the only shape of file name the download endpoint will serve,
// which is what keeps a crafted path out of the filesystem.
var backupNameRE = regexp.MustCompile(`^knot-\d{8}-\d{6}\.zip$`)

// handleAdminBackups lists the archives on disk.
func (s *Server) handleAdminBackups(w http.ResponseWriter, r *http.Request) {
	if s.mustAdmin(w, r) == nil {
		return
	}
	archives, err := backup.List(s.cfg.BackupDir)
	if err != nil {
		writeStoreError(s, w, r, err, "could not read the backup directory")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"archives": archives,
		"keep":     s.cfg.BackupKeep,
		"dir":      s.cfg.BackupDir,
	})
}

// handleAdminCreateBackup writes a snapshot immediately and returns it.
func (s *Server) handleAdminCreateBackup(w http.ResponseWriter, r *http.Request) {
	admin := s.mustAdmin(w, r)
	if admin == nil {
		return
	}
	if !s.allow(w, r, "backup", 10, 3) {
		return
	}

	archive, err := s.createBackup(r.Context())
	if err != nil {
		s.log.Error("create backup", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not create the backup: "+err.Error())
		return
	}
	_ = s.store.Log(r.Context(), admin.ID, "admin.backup", "backup", 0, archive.Name)
	writeJSON(w, http.StatusCreated, map[string]any{"archive": archive})
}

// handleAdminDownloadBackup serves an archive from the backup directory.
func (s *Server) handleAdminDownloadBackup(w http.ResponseWriter, r *http.Request) {
	if s.mustAdmin(w, r) == nil {
		return
	}

	name := r.PathValue("name")
	if !backupNameRE.MatchString(name) {
		writeError(w, http.StatusBadRequest, "bad_request", "invalid backup name")
		return
	}

	path := filepath.Join(s.cfg.BackupDir, name)
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "no such backup")
		return
	}

	// A backup contains password hashes and every private message body in the
	// outbox; it must never be cached by a proxy or a browser.
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	http.ServeFile(w, r, path)
}

// createBackup writes one archive and returns its description.
func (s *Server) createBackup(ctx context.Context) (backup.Archive, error) {
	now := time.Now()
	name := backup.Name(now)
	size, err := backup.Create(ctx, s.store.DB(), s.cfg.UploadDir,
		filepath.Join(s.cfg.BackupDir, name), Version)
	if err != nil {
		return backup.Archive{}, err
	}
	return backup.Archive{Name: name, Size: size, CreatedAt: now.Unix()}, nil
}

// maybeBackup takes a snapshot at most once a day, and prunes old ones.
//
// It runs on the housekeeping tick rather than on a second timer: a self-hosted
// forum does not need precise backup scheduling, and one loop is easier to
// reason about than two.
func (s *Server) maybeBackup(ctx context.Context) {
	if s.cfg.BackupKeep <= 0 {
		return
	}

	archives, err := backup.List(s.cfg.BackupDir)
	if err != nil {
		s.log.Warn("list backups", "err", err)
		return
	}
	if len(archives) > 0 && time.Since(time.Unix(archives[0].CreatedAt, 0)) < 23*time.Hour {
		return
	}

	archive, err := s.createBackup(ctx)
	if err != nil {
		s.log.Error("scheduled backup failed", "err", err)
		return
	}
	s.log.Info("backup written", "file", archive.Name, "bytes", archive.Size)

	if err := backup.Prune(s.cfg.BackupDir, s.cfg.BackupKeep); err != nil {
		s.log.Warn("prune backups", "err", err)
	}
}
