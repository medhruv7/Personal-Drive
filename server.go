package main

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const tempPrefix = ".upload-"

type Server struct {
	root       *os.Root
	rootName   string // display name of the root folder (used for zip names)
	showHidden bool
	maxUpload  int64 // bytes per request; 0 = unlimited
	static     fs.FS
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/list", s.handleList)
	mux.HandleFunc("GET /api/download", s.handleDownload)
	mux.HandleFunc("GET /api/zip", s.handleZip)
	mux.HandleFunc("POST /api/upload", s.handleUpload)
	mux.HandleFunc("POST /api/mkdir", s.handleMkdir)

	files := http.FileServerFS(s.static)
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	}))

	// Middleware chain. Authentication, when added, belongs here — wrap mux
	// before logging so every request, including static files, is covered.
	return withLogging(withRecover(mux))
}

// ---- list ------------------------------------------------------------------

type entry struct {
	Name    string    `json:"name"`
	IsDir   bool      `json:"isDir"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	rel, err := resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, err)
		return
	}
	dir, err := s.root.Open(rel)
	if err != nil {
		writeError(w, err)
		return
	}
	defer dir.Close()
	if info, err := dir.Stat(); err != nil {
		writeError(w, err)
		return
	} else if !info.IsDir() {
		writeError(w, badRequest("not a folder"))
		return
	}
	dirEntries, err := dir.ReadDir(-1)
	if err != nil {
		writeError(w, err)
		return
	}

	entries := make([]entry, 0, len(dirEntries))
	for _, de := range dirEntries {
		name := de.Name()
		if strings.HasPrefix(name, tempPrefix) || (!s.showHidden && isHidden(name)) {
			continue
		}
		// Stat through the root so symlinks are followed only if they stay
		// inside it; escaping or broken links are simply not listed.
		info, err := s.root.Stat(filepath.Join(rel, name))
		if err != nil {
			continue
		}
		e := entry{Name: name, IsDir: info.IsDir(), ModTime: info.ModTime().UTC()}
		if !e.IsDir {
			e.Size = info.Size()
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].IsDir != entries[j].IsDir {
			return entries[i].IsDir
		}
		return strings.ToLower(entries[i].Name) < strings.ToLower(entries[j].Name)
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"path":    canonicalPath(filepath.ToSlash(rel)),
		"entries": entries,
	})
}

// ---- download --------------------------------------------------------------

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	rel, err := resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, err)
		return
	}
	f, err := s.root.Open(rel)
	if err != nil {
		writeError(w, err)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		writeError(w, err)
		return
	}
	if info.IsDir() {
		writeError(w, badRequest("is a folder; use /api/zip"))
		return
	}
	setAttachment(w, info.Name())
	// ServeContent handles Range/If-Modified-Since, so downloads can resume
	// and phones can seek within videos.
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func setAttachment(w http.ResponseWriter, name string) {
	// FormatMediaType emits RFC 2231 filename*=utf-8''... for non-ASCII names.
	if v := mime.FormatMediaType("attachment", map[string]string{"filename": name}); v != "" {
		w.Header().Set("Content-Disposition", v)
	} else {
		w.Header().Set("Content-Disposition", "attachment")
	}
}

// ---- zip -------------------------------------------------------------------

// Formats that are already compressed; deflating them again wastes CPU.
var storedExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true, ".heic": true,
	".mp4": true, ".mov": true, ".mkv": true, ".avi": true, ".webm": true,
	".mp3": true, ".aac": true, ".m4a": true, ".flac": true, ".ogg": true,
	".zip": true, ".gz": true, ".tgz": true, ".bz2": true, ".xz": true, ".7z": true, ".rar": true,
	".pdf": true, ".docx": true, ".xlsx": true, ".pptx": true,
}

func (s *Server) handleZip(w http.ResponseWriter, r *http.Request) {
	rel, err := resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, err)
		return
	}
	info, err := s.root.Stat(rel)
	if err != nil {
		writeError(w, err)
		return
	}
	if !info.IsDir() {
		writeError(w, badRequest("not a folder"))
		return
	}

	base := s.rootName
	if rel != "." {
		base = filepath.Base(rel)
	}
	w.Header().Set("Content-Type", "application/zip")
	setAttachment(w, base+".zip")

	fsys := s.root.FS()
	start := filepath.ToSlash(rel)
	zw := zip.NewWriter(w)
	err = fs.WalkDir(fsys, start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries rather than abort mid-stream
		}
		if p != start && !s.showHidden && isHidden(d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), tempPrefix) {
			return nil
		}
		name := base
		switch {
		case p == start:
		case start == ".":
			name = path.Join(base, p)
		default:
			name = path.Join(base, strings.TrimPrefix(p, start+"/"))
		}
		if d.IsDir() {
			hdr := &zip.FileHeader{Name: name + "/"}
			if fi, err := d.Info(); err == nil {
				hdr.Modified = fi.ModTime()
			}
			_, err := zw.CreateHeader(hdr)
			return err
		}
		if !d.Type().IsRegular() {
			return nil // skip symlinks, devices, etc.
		}
		fi, err := d.Info()
		if err != nil {
			return nil
		}
		hdr, err := zip.FileInfoHeader(fi)
		if err != nil {
			return nil
		}
		hdr.Name = name
		hdr.Method = zip.Deflate
		if storedExts[strings.ToLower(path.Ext(p))] {
			hdr.Method = zip.Store
		}
		dst, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		src, err := fsys.Open(p)
		if err != nil {
			return nil
		}
		defer src.Close()
		_, err = io.Copy(dst, src)
		return err
	})
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		// Headers are already sent; the truncated zip signals failure.
		log.Printf("zip %q: %v", rel, err)
	}
}

// ---- upload ----------------------------------------------------------------

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	rel, err := resolvePath(r.URL.Query().Get("path"))
	if err != nil {
		writeError(w, err)
		return
	}
	if info, err := s.root.Stat(rel); err != nil {
		writeError(w, err)
		return
	} else if !info.IsDir() {
		writeError(w, badRequest("not a folder"))
		return
	}
	if s.maxUpload > 0 {
		r.Body = http.MaxBytesReader(w, r.Body, s.maxUpload)
	}
	mr, err := r.MultipartReader()
	if err != nil {
		writeError(w, badRequest("expected multipart/form-data"))
		return
	}

	var saved []string
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			writeError(w, uploadErr(err))
			return
		}
		if part.FormName() != "files" || part.FileName() == "" {
			part.Close()
			continue
		}
		name := uploadName(part.FileName())
		if err := validateName(name); err != nil {
			part.Close()
			writeError(w, err)
			return
		}
		final, err := s.saveFile(rel, name, part)
		part.Close()
		if err != nil {
			writeError(w, uploadErr(err))
			return
		}
		saved = append(saved, final)
	}
	if len(saved) == 0 {
		writeError(w, badRequest("no files in request"))
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"uploaded": saved})
}

// saveFile streams src into a temp file inside dir, then renames it to a
// name that does not collide with anything existing ("name (1).ext", ...).
func (s *Server) saveFile(dir, name string, src io.Reader) (string, error) {
	tmpName := filepath.Join(dir, tempPrefix+randomHex(8)+".tmp")
	tmp, err := s.root.OpenFile(tmpName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", err
	}
	_, copyErr := io.Copy(tmp, src)
	closeErr := tmp.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		s.root.Remove(tmpName)
		return "", err
	}

	final, err := s.reserveName(dir, name)
	if err != nil {
		s.root.Remove(tmpName)
		return "", err
	}
	if err := s.root.Rename(tmpName, filepath.Join(dir, final)); err != nil {
		s.root.Remove(tmpName)
		s.root.Remove(filepath.Join(dir, final))
		return "", err
	}
	return final, nil
}

// reserveName atomically claims a free name in dir by creating an empty
// placeholder with O_EXCL, which the finished upload is then renamed over.
func (s *Server) reserveName(dir, name string) (string, error) {
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" { // e.g. ".bashrc"
		stem, ext = name, ""
	}
	for i := 0; i < 10000; i++ {
		candidate := name
		if i > 0 {
			candidate = fmt.Sprintf("%s (%d)%s", stem, i, ext)
		}
		f, err := s.root.OpenFile(filepath.Join(dir, candidate), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			f.Close()
			return candidate, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("too many files with the same name")
}

func uploadErr(err error) error {
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) {
		return httpError{http.StatusRequestEntityTooLarge, fmt.Sprintf("upload exceeds %d bytes", tooBig.Limit)}
	}
	return err
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ---- mkdir -----------------------------------------------------------------

func (s *Server) handleMkdir(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, badRequest("invalid JSON body"))
		return
	}
	rel, err := resolvePath(req.Path)
	if err != nil {
		writeError(w, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := validateName(req.Name); err != nil {
		writeError(w, err)
		return
	}
	if err := s.root.Mkdir(filepath.Join(rel, req.Name), 0o755); err != nil {
		if errors.Is(err, fs.ErrExist) {
			err = httpError{http.StatusConflict, "a file or folder with that name already exists"}
		}
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"name": req.Name})
}

// ---- responses & middleware -----------------------------------------------

type httpError struct {
	status int
	msg    string
}

func (e httpError) Error() string { return e.msg }

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status, msg := http.StatusInternalServerError, "internal error"
	var he httpError
	switch {
	case errors.As(err, &he):
		status, msg = he.status, he.msg
	case errors.Is(err, errBadRequest):
		status, msg = http.StatusBadRequest, err.Error()
	case errors.Is(err, fs.ErrNotExist):
		status, msg = http.StatusNotFound, "not found"
	case errors.Is(err, fs.ErrExist):
		status, msg = http.StatusConflict, "already exists"
	case errors.Is(err, fs.ErrPermission):
		status, msg = http.StatusForbidden, "permission denied"
	case strings.Contains(err.Error(), "escapes from parent"):
		// os.Root refused a symlink pointing outside the drive.
		status, msg = http.StatusForbidden, "access outside the drive is not allowed"
	default:
		log.Printf("error: %v", err)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Unwrap() http.ResponseWriter { return r.ResponseWriter }

func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s %s %d %s", r.RemoteAddr, r.Method, r.URL.RequestURI(), rec.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

func withRecover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Printf("panic: %v", v)
				writeError(w, fmt.Errorf("panic: %v", v))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
