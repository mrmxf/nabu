package webfetch

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tarball builds a gzipped tar from name -> content; a "/" suffix is a folder,
// and a content of "\x00link" makes a symlink.
func tarball(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range files {
		h := &tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}
		switch {
		case strings.HasSuffix(name, "/"):
			h.Typeflag, h.Size, h.Mode = tar.TypeDir, 0, 0o755
		case body == "\x00link":
			h.Typeflag, h.Size, h.Linkname = tar.TypeSymlink, 0, "/etc/passwd"
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			if _, err := tw.Write([]byte(body)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// release serves <version>/checksums.txt and <version>/nabu-web-<version>.tar.gz.
// sum overrides the archive's listed sha256 when non-empty.
func release(t *testing.T, version string, archive []byte, sum string) (*httptest.Server, *int) {
	t.Helper()
	if sum == "" {
		s := sha256.Sum256(archive)
		sum = hex.EncodeToString(s[:])
	}
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		switch r.URL.Path {
		case "/" + version + "/checksums.txt":
			_, _ = fmt.Fprintf(w, "%s  nabu-amd-lnx\n%s  %s\n", strings.Repeat("0", 64), sum, AssetName(version))
		case "/" + version + "/" + AssetName(version):
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func goMod(t *testing.T, require string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "go.mod")
	body := "module example.nabu/course\n\ngo 1.26\n\nrequire (\n\tgithub.com/spf13/cobra v1.10.1 // indirect\n\t" + require + "\n)\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

var app = map[string]string{
	"./":                          "",
	"./index.html":                "<!doctype html>",
	"./_app/immutable/":           "",
	"./_app/immutable/entry.js":   "export{}",
	"./_app/immutable/assets/a.c": "body{}",
}

func TestResolveVersion(t *testing.T) {
	for _, tc := range []struct {
		name, flag, require, want, src, errHas string
	}{
		{"flag wins", "v0.3.0", "github.com/mrmxf/nabu v0.1.2", "v0.3.0", "flag", ""},
		{"from go.mod", "", "github.com/mrmxf/nabu v0.1.2", "v0.1.2", "go.mod", ""},
		{"pseudo-version refused", "", "github.com/mrmxf/nabu v0.1.3-0.20261114171200-44ecdb3a1b2c", "", "", "not a release"},
		{"bad flag", "latest", "", "", "", "not a release tag"},
		{"nothing to go on", "", "github.com/other/mod v1.0.0", "", "", "Pass --version"},
	} {
		v, src, err := ResolveVersion(tc.flag, goMod(t, tc.require))
		if tc.errHas != "" {
			if err == nil || !strings.Contains(err.Error(), tc.errHas) {
				t.Errorf("%s: err = %v, want it to contain %q", tc.name, err, tc.errHas)
			}
			continue
		}
		if err != nil || v != tc.want || src != tc.src {
			t.Errorf("%s: got %q from %q (err %v), want %q from %q", tc.name, v, src, err, tc.want, tc.src)
		}
	}
}

func TestFetchVerifiesAndInstalls(t *testing.T) {
	srv, hits := release(t, "v0.1.2", tarball(t, app), "")
	dir := filepath.Join(t.TempDir(), "nabu-web")
	o := Options{Dir: dir, BaseURL: srv.URL, GoMod: goMod(t, "github.com/mrmxf/nabu v0.1.2")}

	res, err := Fetch(context.Background(), o)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !res.Fetched || res.Version != "v0.1.2" || res.Source != "go.mod" || res.Files != 3 {
		t.Errorf("result = %+v", res)
	}
	for _, f := range []string{"index.html", "_app/immutable/entry.js", MarkerFile} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s", f)
		}
	}

	// a second run with the same release is offline and changes nothing
	before := *hits
	res, err = Fetch(context.Background(), o)
	if err != nil || res.Fetched || *hits != before {
		t.Errorf("second Fetch: fetched=%v hits %d -> %d err=%v; want no download", res.Fetched, before, *hits, err)
	}

	// --force replaces it, and leaves no temporary folders behind
	o.Force = true
	if _, err := Fetch(context.Background(), o); err != nil {
		t.Fatalf("forced Fetch: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 1 {
		t.Errorf("parent holds %d entries, want only %s", len(entries), filepath.Base(dir))
	}
}

func TestFetchRefusesABadChecksum(t *testing.T) {
	srv, _ := release(t, "v0.1.2", tarball(t, app), strings.Repeat("ab", 32))
	dir := filepath.Join(t.TempDir(), "nabu-web")
	_, err := Fetch(context.Background(), Options{Dir: dir, BaseURL: srv.URL, Version: "v0.1.2"})
	if err == nil || !strings.Contains(err.Error(), "does not match checksums.txt") {
		t.Fatalf("err = %v, want a checksum refusal", err)
	}
	if _, err := os.Stat(dir); err == nil {
		t.Error("a rejected archive was unpacked")
	}
}

func TestFetchRefusesUnsafeArchives(t *testing.T) {
	for name, extra := range map[string]map[string]string{
		"parent path": {"../escape.txt": "x"},
		"symlink":     {"./evil": "\x00link"},
	} {
		files := map[string]string{"./index.html": "<!doctype html>"}
		for k, v := range extra {
			files[k] = v
		}
		srv, _ := release(t, "v0.1.2", tarball(t, files), "")
		dir := filepath.Join(t.TempDir(), "nabu-web")
		_, err := Fetch(context.Background(), Options{Dir: dir, BaseURL: srv.URL, Version: "v0.1.2"})
		if err == nil || !strings.Contains(err.Error(), "refusing") {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "escape.txt")); err == nil {
			t.Errorf("%s: wrote outside the target", name)
		}
	}
}

func TestFetchNeverReplacesAForeignFolder(t *testing.T) {
	srv, _ := release(t, "v0.1.2", tarball(t, app), "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Fetch(context.Background(), Options{Dir: dir, BaseURL: srv.URL, Version: "v0.1.2"})
	if err == nil || !strings.Contains(err.Error(), "was not written by nabu web fetch") {
		t.Fatalf("err = %v, want a refusal", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "notes.txt")); string(b) != "mine" {
		t.Error("the folder was changed")
	}
}

func TestFetchMissingAsset(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  nabu-amd-lnx\n", strings.Repeat("0", 64))
	}))
	t.Cleanup(srv.Close)
	_, err := Fetch(context.Background(), Options{Dir: filepath.Join(t.TempDir(), "w"), BaseURL: srv.URL, Version: "v0.1.0"})
	if err == nil || !strings.Contains(err.Error(), "has no web app asset") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchUnpublishedRelease(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	_, err := Fetch(context.Background(), Options{Dir: filepath.Join(t.TempDir(), "w"), BaseURL: srv.URL, Version: "v0.1.1"})
	if err == nil || !strings.Contains(err.Error(), "no published release v0.1.1") {
		t.Fatalf("err = %v", err)
	}
}
