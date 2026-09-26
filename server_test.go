package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func newTestServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	static, _ := fs.Sub(webFiles, "web")
	s := &Server{root: root, rootName: "Drive", static: static}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(t *testing.T, ts *httptest.Server, endpoint, p string) *http.Response {
	t.Helper()
	res, err := http.Get(ts.URL + endpoint + "?path=" + url.QueryEscape(p))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

type listResp struct {
	Path    string  `json:"path"`
	Entries []entry `json:"entries"`
}

func list(t *testing.T, ts *httptest.Server, p string) listResp {
	t.Helper()
	res := get(t, ts, "/api/list", p)
	if res.StatusCode != 200 {
		t.Fatalf("list %q: status %d", p, res.StatusCode)
	}
	var lr listResp
	if err := json.NewDecoder(res.Body).Decode(&lr); err != nil {
		t.Fatal(err)
	}
	return lr
}

func names(lr listResp) []string {
	var out []string
	for _, e := range lr.Entries {
		out = append(out, e.Name)
	}
	return out
}

func upload(t *testing.T, ts *httptest.Server, dir string, files map[string]string) *http.Response {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	for name, content := range files {
		fw, err := mw.CreateFormFile("files", name)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(fw, content)
	}
	mw.Close()
	res, err := http.Post(ts.URL+"/api/upload?path="+url.QueryEscape(dir), mw.FormDataContentType(), &body)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func mkdir(t *testing.T, ts *httptest.Server, dir, name string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"path": dir, "name": name})
	res, err := http.Post(ts.URL+"/api/mkdir", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestListSortsFoldersFirstAndHidesHidden(t *testing.T) {
	ts, dir := newTestServer(t)
	writeFile(t, filepath.Join(dir, "b.txt"), "b")
	writeFile(t, filepath.Join(dir, "A.txt"), "a")
	writeFile(t, filepath.Join(dir, ".secret"), "x")
	writeFile(t, filepath.Join(dir, "zeta", "inner.txt"), "i")

	lr := list(t, ts, "/")
	if got, want := names(lr), []string{"zeta", "A.txt", "b.txt"}; !slices.Equal(got, want) {
		t.Fatalf("entries = %v, want %v", got, want)
	}
	if lr.Path != "/" || !lr.Entries[0].IsDir || lr.Entries[1].Size != 1 {
		t.Fatalf("unexpected response %+v", lr)
	}
	if got := names(list(t, ts, "/zeta")); !slices.Equal(got, []string{"inner.txt"}) {
		t.Fatalf("zeta entries = %v", got)
	}
	if res := get(t, ts, "/api/list", "/missing"); res.StatusCode != 404 {
		t.Fatalf("missing folder: status %d, want 404", res.StatusCode)
	}
	if res := get(t, ts, "/api/list", "/b.txt"); res.StatusCode != 400 {
		t.Fatalf("list of file: status %d, want 400", res.StatusCode)
	}
}

func TestMkdir(t *testing.T) {
	ts, dir := newTestServer(t)
	if res := mkdir(t, ts, "/", "Photos"); res.StatusCode != 201 {
		t.Fatalf("mkdir: status %d", res.StatusCode)
	}
	if res := mkdir(t, ts, "/Photos", "2024"); res.StatusCode != 201 {
		t.Fatalf("nested mkdir: status %d", res.StatusCode)
	}
	if fi, err := os.Stat(filepath.Join(dir, "Photos", "2024")); err != nil || !fi.IsDir() {
		t.Fatalf("folder not created: %v", err)
	}
	if res := mkdir(t, ts, "/", "Photos"); res.StatusCode != 409 {
		t.Fatalf("duplicate mkdir: status %d, want 409", res.StatusCode)
	}
	for _, bad := range []string{"", "..", "a/b", `a\b`, "con", "x.", "what?"} {
		if res := mkdir(t, ts, "/", bad); res.StatusCode != 400 {
			t.Errorf("mkdir %q: status %d, want 400", bad, res.StatusCode)
		}
	}
	if res := mkdir(t, ts, "/nope", "x"); res.StatusCode != 404 {
		t.Fatalf("mkdir in missing parent: status %d, want 404", res.StatusCode)
	}
}

func TestUploadAutoRenames(t *testing.T) {
	ts, dir := newTestServer(t)
	writeFile(t, filepath.Join(dir, "report.pdf"), "original")

	res := upload(t, ts, "/", map[string]string{"report.pdf": "new", "notes.txt": "hello"})
	if res.StatusCode != 201 {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("upload: status %d: %s", res.StatusCode, b)
	}
	var out struct{ Uploaded []string }
	json.NewDecoder(res.Body).Decode(&out)
	slices.Sort(out.Uploaded)
	if want := []string{"notes.txt", "report (1).pdf"}; !slices.Equal(out.Uploaded, want) {
		t.Fatalf("uploaded = %v, want %v", out.Uploaded, want)
	}
	check := map[string]string{"report.pdf": "original", "report (1).pdf": "new", "notes.txt": "hello"}
	for name, want := range check {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(b) != want {
			t.Errorf("%s = %q, %v; want %q", name, b, err, want)
		}
	}
	// No temp files left behind.
	des, _ := os.ReadDir(dir)
	for _, de := range des {
		if strings.HasPrefix(de.Name(), tempPrefix) {
			t.Errorf("leftover temp file %s", de.Name())
		}
	}
}

func TestUploadStripsClientPathAndValidates(t *testing.T) {
	ts, dir := newTestServer(t)
	if res := upload(t, ts, "/", map[string]string{`C:\Users\me\photo.jpg`: "img"}); res.StatusCode != 201 {
		t.Fatalf("upload with windows path: status %d", res.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, "photo.jpg")); err != nil {
		t.Fatalf("photo.jpg not saved: %v", err)
	}
	if res := upload(t, ts, "/", map[string]string{"bad|name.txt": "x"}); res.StatusCode != 400 {
		t.Fatalf("invalid name: status %d, want 400", res.StatusCode)
	}
	if res := upload(t, ts, "/missing", map[string]string{"a.txt": "x"}); res.StatusCode != 404 {
		t.Fatalf("upload to missing folder: status %d, want 404", res.StatusCode)
	}
}

func TestUploadSizeLimit(t *testing.T) {
	dir := t.TempDir()
	root, _ := os.OpenRoot(dir)
	defer root.Close()
	ts := httptest.NewServer((&Server{root: root, maxUpload: 1024}).Handler())
	defer ts.Close()

	if res := upload(t, ts, "/", map[string]string{"big.bin": strings.Repeat("x", 4096)}); res.StatusCode != 413 {
		t.Fatalf("oversized upload: status %d, want 413", res.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(dir, "big.bin")); err == nil {
		t.Fatal("oversized upload was saved")
	}
	des, _ := os.ReadDir(dir)
	if len(des) != 0 {
		t.Fatalf("leftover files after failed upload: %v", des)
	}
}

func TestDownloadAndRange(t *testing.T) {
	ts, dir := newTestServer(t)
	writeFile(t, filepath.Join(dir, "docs", "résumé.txt"), "0123456789")

	res := get(t, ts, "/api/download", "/docs/résumé.txt")
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(body) != "0123456789" {
		t.Fatalf("download: %d %q", res.StatusCode, body)
	}
	if cd := res.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") || !strings.Contains(cd, "utf-8''r%C3%A9sum%C3%A9.txt") {
		t.Fatalf("Content-Disposition = %q", cd)
	}

	req, _ := http.NewRequest("GET", ts.URL+"/api/download?path="+url.QueryEscape("/docs/résumé.txt"), nil)
	req.Header.Set("Range", "bytes=2-5")
	rres, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer rres.Body.Close()
	rbody, _ := io.ReadAll(rres.Body)
	if rres.StatusCode != 206 || string(rbody) != "2345" {
		t.Fatalf("range: %d %q", rres.StatusCode, rbody)
	}

	if res := get(t, ts, "/api/download", "/docs"); res.StatusCode != 400 {
		t.Fatalf("download folder: status %d, want 400", res.StatusCode)
	}
}

func TestZip(t *testing.T) {
	ts, dir := newTestServer(t)
	writeFile(t, filepath.Join(dir, "album", "a.txt"), "A")
	writeFile(t, filepath.Join(dir, "album", "sub", "b.jpg"), "B")
	writeFile(t, filepath.Join(dir, "album", ".DS_Store"), "junk")

	check := func(p string, want map[string]string) {
		t.Helper()
		res := get(t, ts, "/api/zip", p)
		if res.StatusCode != 200 {
			t.Fatalf("zip %q: status %d", p, res.StatusCode)
		}
		data, _ := io.ReadAll(res.Body)
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, f := range zr.File {
			if strings.HasSuffix(f.Name, "/") {
				continue
			}
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			rc.Close()
			got[f.Name] = string(b)
		}
		if len(got) != len(want) {
			t.Fatalf("zip %q contents = %v, want %v", p, got, want)
		}
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("zip %q contents = %v, want %v", p, got, want)
			}
		}
	}
	check("/album", map[string]string{"album/a.txt": "A", "album/sub/b.jpg": "B"})
	check("/", map[string]string{"Drive/album/a.txt": "A", "Drive/album/sub/b.jpg": "B"})
}

func TestPathTraversalRejected(t *testing.T) {
	ts, dir := newTestServer(t)
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "secret.txt"), "TOP SECRET")
	writeFile(t, filepath.Join(dir, "ok.txt"), "fine")

	// Lexical traversal is cleaned back to the root, so these must never
	// reach the outside file.
	for _, p := range []string{"../secret.txt", "/../../secret.txt", "..%2fsecret.txt", filepath.Join(outside, "secret.txt"), `..\secret.txt`} {
		res := get(t, ts, "/api/download", p)
		body, _ := io.ReadAll(res.Body)
		if strings.Contains(string(body), "TOP SECRET") {
			t.Errorf("download %q leaked outside file", p)
		}
		if res.StatusCode == 200 {
			t.Errorf("download %q: status 200, want error", p)
		}
	}
	// "../ok.txt" resolves to the in-root file, which is fine.
	if res := get(t, ts, "/api/download", "../ok.txt"); res.StatusCode != 200 {
		t.Errorf("cleaned path: status %d", res.StatusCode)
	}

	if runtime.GOOS == "windows" {
		return // creating symlinks needs extra privileges on Windows
	}
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(dir, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ endpoint, path string }{
		{"/api/download", "/escape/secret.txt"},
		{"/api/download", "/escape.txt"},
		{"/api/list", "/escape"},
		{"/api/zip", "/escape"},
	} {
		res := get(t, ts, c.endpoint, c.path)
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode == 200 || strings.Contains(string(body), "TOP SECRET") {
			t.Errorf("%s %q through symlink: status %d", c.endpoint, c.path, res.StatusCode)
		}
	}
	if res := upload(t, ts, "/escape", map[string]string{"x.txt": "x"}); res.StatusCode < 400 {
		t.Errorf("upload through symlink: status %d", res.StatusCode)
	}
	if _, err := os.Stat(filepath.Join(outside, "x.txt")); err == nil {
		t.Error("upload wrote outside the root")
	}
	// Escaping symlinks are not listed, and the root zip does not follow them.
	if got := names(list(t, ts, "/")); slices.Contains(got, "escape") || slices.Contains(got, "escape.txt") {
		t.Errorf("escaping symlinks listed: %v", got)
	}
	res := get(t, ts, "/api/zip", "/")
	body, _ := io.ReadAll(res.Body)
	if bytes.Contains(body, []byte("TOP SECRET")) {
		t.Error("root zip included file from outside the root")
	}
}

func TestStaticUI(t *testing.T) {
	ts, _ := newTestServer(t)
	for _, p := range []string{"/", "/app.js", "/style.css"} {
		res, err := http.Get(ts.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 200 {
			t.Errorf("GET %s: status %d", p, res.StatusCode)
		}
	}
}
