// Package backup writes a self-contained snapshot of an instance.
//
// A backup is a zip holding a consistent copy of the database and the uploaded
// files, plus a manifest so a year-old archive explains itself.
package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manifest is written into the archive.
type Manifest struct {
	Version   string `json:"version"`
	CreatedAt int64  `json:"created_at"`
	Database  string `json:"database"`
	Uploads   int    `json:"uploads"`
}

// Name returns the file name for a backup taken at t, in UTC so that archives
// sort in the order they were made.
func Name(t time.Time) string {
	return "knot-" + t.UTC().Format("20060102-150405") + ".zip"
}

// Create writes a snapshot of db and uploadDir to destPath.
//
// The database is copied with VACUUM INTO rather than by reading the file: it
// takes a consistent snapshot while writers continue, and leaves behind a
// compacted database instead of a database plus a WAL file.
func Create(ctx context.Context, db *sql.DB, uploadDir, destPath, version string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(destPath), 0o750); err != nil {
		return 0, fmt.Errorf("create backup directory: %w", err)
	}

	snapshot, err := os.CreateTemp(filepath.Dir(destPath), "snapshot-*.db")
	if err != nil {
		return 0, fmt.Errorf("create snapshot file: %w", err)
	}
	snapshotPath := snapshot.Name()
	snapshot.Close()
	// VACUUM INTO refuses to overwrite an existing file, and it needs a path that
	// does not exist yet.
	os.Remove(snapshotPath)
	defer os.Remove(snapshotPath)

	// The path is generated here, never supplied by a caller, so interpolating it
	// is safe; SQLite's VACUUM INTO does not accept a bound parameter everywhere.
	quoted := "'" + strings.ReplaceAll(snapshotPath, "'", "''") + "'"
	if _, err := db.ExecContext(ctx, "VACUUM INTO "+quoted); err != nil {
		return 0, fmt.Errorf("snapshot database: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("create archive: %w", err)
	}
	defer out.Close()

	zw := zip.NewWriter(out)
	uploads, err := addUploads(ctx, zw, uploadDir)
	if err != nil {
		zw.Close()
		return 0, err
	}

	if err := addFile(zw, "knot.db", snapshotPath); err != nil {
		zw.Close()
		return 0, err
	}

	manifest, err := json.MarshalIndent(Manifest{
		Version:   version,
		CreatedAt: time.Now().Unix(),
		Database:  "knot.db",
		Uploads:   uploads,
	}, "", "  ")
	if err != nil {
		zw.Close()
		return 0, err
	}
	w, err := zw.Create("manifest.json")
	if err != nil {
		zw.Close()
		return 0, err
	}
	if _, err := w.Write(manifest); err != nil {
		zw.Close()
		return 0, err
	}

	if err := zw.Close(); err != nil {
		return 0, fmt.Errorf("finish archive: %w", err)
	}
	if err := out.Close(); err != nil {
		return 0, fmt.Errorf("close archive: %w", err)
	}

	info, err := os.Stat(destPath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// addUploads copies the upload directory into the archive, returning how many
// files were included. A missing directory simply means nothing was uploaded
// yet, which is not an error.
func addUploads(ctx context.Context, zw *zip.Writer, uploadDir string) (int, error) {
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read uploads: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		if entry.IsDir() || !entry.Type().IsRegular() {
			continue
		}
		// Entry names use forward slashes whatever the host separator is, or an
		// archive made on Windows extracts to a file literally called
		// "uploads\picture.png" when it is opened on Linux.
		if err := addFile(zw, path.Join("uploads", entry.Name()), filepath.Join(uploadDir, entry.Name())); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func addFile(zw *zip.Writer, name, path string) error {
	src, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer src.Close()

	w, err := zw.Create(name)
	if err != nil {
		return fmt.Errorf("add %s: %w", name, err)
	}
	if _, err := io.Copy(w, src); err != nil {
		return fmt.Errorf("copy %s: %w", name, err)
	}
	return nil
}

// Archive describes one file in the backup directory.
type Archive struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	CreatedAt int64  `json:"created_at"`
}

// List returns the archives in dir, newest first.
func List(dir string) ([]Archive, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Archive{}, nil
		}
		return nil, err
	}

	out := []Archive{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "knot-") || !strings.HasSuffix(name, ".zip") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, Archive{
			Name:      name,
			Size:      info.Size(),
			CreatedAt: info.ModTime().Unix(),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, nil
}

// Prune keeps the newest `keep` archives and deletes the rest.
func Prune(dir string, keep int) error {
	if keep <= 0 {
		return nil
	}
	archives, err := List(dir)
	if err != nil {
		return err
	}
	for i := keep; i < len(archives); i++ {
		if err := os.Remove(filepath.Join(dir, archives[i].Name)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
