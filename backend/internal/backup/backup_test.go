package backup_test

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sysvo/knot/internal/backup"
)

// TestCreateMakesARestorableArchive reads the archive back and queries the
// database inside it. Asserting that a zip exists would pass even if the copy
// were empty; this fails unless the data actually survives.
func TestCreateMakesARestorableArchive(t *testing.T) {
	dir := t.TempDir()

	dbPath := filepath.Join(dir, "knot.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE posts (id INTEGER PRIMARY KEY, body TEXT NOT NULL)`); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO posts (body) VALUES ('kept across the backup')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}

	uploadDir := filepath.Join(dir, "uploads")
	if err := os.MkdirAll(uploadDir, 0o750); err != nil {
		t.Fatalf("create upload directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(uploadDir, "picture.png"), []byte("not really a png"), 0o600); err != nil {
		t.Fatalf("write upload: %v", err)
	}

	dest := filepath.Join(dir, "backups", backup.Name(time.Now()))
	size, err := backup.Create(context.Background(), db, uploadDir, dest, "v-test")
	if err != nil {
		t.Fatalf("create backup: %v", err)
	}

	info, err := os.Stat(dest)
	if err != nil {
		t.Fatalf("stat archive: %v", err)
	}
	if size != info.Size() || size == 0 {
		t.Fatalf("Create returned %d bytes, file is %d", size, info.Size())
	}

	archive, err := zip.OpenReader(dest)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer archive.Close()

	files := map[string]*zip.File{}
	for _, f := range archive.File {
		files[f.Name] = f
	}

	// Uploads are stored with forward slashes whatever the host separator is.
	for _, want := range []string{"knot.db", "manifest.json", "uploads/picture.png"} {
		if files[want] == nil {
			t.Fatalf("archive has no %q; it holds %v", want, names(files))
		}
	}

	var manifest backup.Manifest
	readZipJSON(t, files["manifest.json"], &manifest)
	if manifest.Version != "v-test" {
		t.Errorf("manifest version = %q, want %q", manifest.Version, "v-test")
	}
	if manifest.Database != "knot.db" {
		t.Errorf("manifest database = %q, want knot.db", manifest.Database)
	}
	if manifest.Uploads != 1 {
		t.Errorf("manifest uploads = %d, want 1", manifest.Uploads)
	}

	restored := filepath.Join(dir, "restored.db")
	readZipFile(t, files["knot.db"], restored)

	restoredDB, err := sql.Open("sqlite", restored)
	if err != nil {
		t.Fatalf("open restored database: %v", err)
	}
	defer restoredDB.Close()

	var body string
	if err := restoredDB.QueryRow(`SELECT body FROM posts`).Scan(&body); err != nil {
		t.Fatalf("read restored row: %v", err)
	}
	if body != "kept across the backup" {
		t.Errorf("restored body = %q", body)
	}
}

// TestCreateWithoutUploads still produces a usable archive.
func TestCreateWithoutUploads(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "knot.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	dest := filepath.Join(dir, "knot-20260101-000000.zip")
	if _, err := backup.Create(context.Background(), db, filepath.Join(dir, "missing"), dest, "v-test"); err != nil {
		t.Fatalf("create backup with no upload directory: %v", err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("stat archive: %v", err)
	}
}

// TestListAndPrune checks the retention rule, not just that files can be listed:
// the two newest archives must survive and the older one must go.
func TestListAndPrune(t *testing.T) {
	dir := t.TempDir()
	names := []string{
		"knot-20260101-000000.zip",
		"knot-20260102-000000.zip",
		"knot-20260103-000000.zip",
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("zip"), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		stamp := base.Add(time.Duration(i) * 24 * time.Hour)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatalf("set times on %s: %v", name, err)
		}
	}
	// A file that is not one of ours must never be counted or pruned.
	other := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(other, []byte("keep me"), 0o600); err != nil {
		t.Fatalf("write notes: %v", err)
	}

	archives, err := backup.List(dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(archives) != 3 {
		t.Fatalf("listed %d archives, want 3", len(archives))
	}
	if archives[0].Name != names[2] {
		t.Errorf("newest archive = %q, want %q", archives[0].Name, names[2])
	}

	if err := backup.Prune(dir, 2); err != nil {
		t.Fatalf("prune: %v", err)
	}

	remaining, err := backup.List(dir)
	if err != nil {
		t.Fatalf("list after prune: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("after pruning to 2, %d archives remain: %v", len(remaining), namesOf(remaining))
	}
	if remaining[0].Name != names[2] || remaining[1].Name != names[1] {
		t.Errorf("kept %v, want the two newest", namesOf(remaining))
	}
	if _, err := os.Stat(other); err != nil {
		t.Errorf("prune removed a file that is not a backup: %v", err)
	}

	// keep = 0 disables pruning rather than deleting everything, because the
	// setting means "do not prune", not "keep none".
	if err := backup.Prune(dir, 0); err != nil {
		t.Fatalf("prune with keep 0: %v", err)
	}
	if left, err := backup.List(dir); err != nil || len(left) != 2 {
		t.Errorf("prune with keep 0 changed the directory: %v %v", left, err)
	}
}

// TestListOfAMissingDirectory is empty rather than an error: a forum that has
// never been backed up has no backup directory, and that is not a fault.
func TestListOfAMissingDirectory(t *testing.T) {
	archives, err := backup.List(filepath.Join(t.TempDir(), "absent"))
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(archives) != 0 {
		t.Errorf("listed %d archives in a directory that does not exist", len(archives))
	}
}

func TestName(t *testing.T) {
	// Same instant in two zones must give the same name, or archives sort wrong
	// depending on where the server runs.
	a := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)
	b := a.In(time.FixedZone("elsewhere", -5*60*60))
	if got := backup.Name(a); got != "knot-20260304-050607.zip" {
		t.Errorf("Name = %q", got)
	}
	if backup.Name(a) != backup.Name(b) {
		t.Errorf("Name depends on the location: %q vs %q", backup.Name(a), backup.Name(b))
	}
}

func names(files map[string]*zip.File) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func namesOf(archives []backup.Archive) []string {
	out := make([]string, 0, len(archives))
	for _, a := range archives {
		out = append(out, a.Name)
	}
	return out
}

func readZipJSON(t *testing.T, f *zip.File, into any) {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Fatalf("open %s: %v", f.Name, err)
	}
	defer rc.Close()
	if err := json.NewDecoder(rc).Decode(into); err != nil {
		t.Fatalf("decode %s: %v", f.Name, err)
	}
}

func readZipFile(t *testing.T, f *zip.File, dest string) {
	t.Helper()
	rc, err := f.Open()
	if err != nil {
		t.Fatalf("open %s: %v", f.Name, err)
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		t.Fatalf("create %s: %v", dest, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, rc); err != nil {
		t.Fatalf("extract %s: %v", f.Name, err)
	}
}
