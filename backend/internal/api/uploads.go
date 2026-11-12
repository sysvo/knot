package api

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// allowedUploads maps a sniffed content type to the extension we store it with.
// The extension comes from the bytes, never from the client's file name.
var allowedUploads = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// handleUpload accepts one image as multipart/form-data under the field "file".
func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	if u == nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "sign in to upload files")
		return
	}
	if !s.allow(w, r, "upload", 20, 10) {
		return
	}

	limit := s.cfg.MaxUploadSize
	r.Body = http.MaxBytesReader(w, r.Body, limit+1024)

	if err := r.ParseMultipartForm(limit); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "that file is too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "attach a file in the \"file\" field")
		return
	}
	defer file.Close()

	if header.Size > limit {
		writeError(w, http.StatusRequestEntityTooLarge, "too_large", "that file is too large")
		return
	}

	// Sniff the real type from the first bytes.
	head := make([]byte, 512)
	n, err := io.ReadFull(file, head)
	if err != nil && err != io.ErrUnexpectedEOF {
		writeError(w, http.StatusBadRequest, "bad_request", "could not read the file")
		return
	}
	head = head[:n]

	contentType := http.DetectContentType(head)
	ext, ok := allowedUploads[contentType]
	if !ok {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_type",
			"only PNG, JPEG, GIF and WebP images can be uploaded")
		return
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read the file")
		return
	}

	name, err := randomBytes(18)
	if err != nil {
		s.log.Error("generate upload name", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not store the file")
		return
	}
	name += ext

	if err := os.MkdirAll(s.cfg.UploadDir, 0o750); err != nil {
		s.log.Error("create upload dir", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not store the file")
		return
	}

	path := filepath.Join(s.cfg.UploadDir, name)
	dst, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		s.log.Error("create upload file", "err", err)
		writeError(w, http.StatusInternalServerError, "internal", "could not store the file")
		return
	}

	written, copyErr := io.Copy(dst, io.LimitReader(file, limit))
	closeErr := dst.Close()
	if copyErr != nil || closeErr != nil {
		os.Remove(path)
		s.log.Error("write upload", "err", copyErr, "close_err", closeErr)
		writeError(w, http.StatusInternalServerError, "internal", "could not store the file")
		return
	}

	upload, err := s.store.CreateUpload(r.Context(), u.ID, name, sanitiseFilename(header.Filename), contentType, written)
	if err != nil {
		os.Remove(path)
		writeStoreError(s, w, r, err, "could not record the upload")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"upload": upload})
}

// sanitiseFilename keeps a readable original name for the record only. It is
// never used as a path.
func sanitiseFilename(name string) string {
	if name == "" {
		return "upload"
	}
	if base := filepath.Base(name); base != "." && base != string(filepath.Separator) {
		name = base
	}
	runes := []rune(name)
	if len(runes) > 120 {
		name = string(runes[:120])
	}
	return name
}
