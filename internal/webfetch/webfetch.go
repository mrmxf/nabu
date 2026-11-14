// Package webfetch downloads Nabu's built web app for a library-form course
// (`nabu web fetch [dir]`).
//
// The web app is a build output (web/, `npm run build`), not part of the Go
// module, so a course that builds its own binary with Nabu as a library does
// not get it from `go get`. Each Nabu release attaches nabu-web-<version>.tar.gz
// and lists its sha256 in checksums.txt; this package picks the release that
// matches the course, verifies the archive against checksums.txt, and unpacks it
// into a folder the course passes as nabu.Options.Public.
package webfetch

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"strings"
	"time"

	"github.com/mrmxf/util/buildinfo"
)

// ModulePath is Nabu's Go module path: the one a course's go.mod requires.
const ModulePath = "github.com/mrmxf/nabu"

// DefaultBaseURL is where release assets are downloaded from:
// <base>/<version>/<asset>.
const DefaultBaseURL = "https://github.com/mrmxf/nabu/releases/download"

// DefaultDir is the folder the web app is unpacked into when none is given.
const DefaultDir = "nabu-web"

// MarkerFile, inside the unpacked folder, records which release it holds. It
// also marks the folder as ours: an existing folder without it is never replaced.
const MarkerFile = ".nabu-web-version"

// maxArchive bounds a download; the web app is well under 10 MB.
const maxArchive = 200 << 20

// Options configures one fetch.
type Options struct {
	Dir     string // target folder; DefaultDir when empty
	Version string // release tag, e.g. v0.1.2; resolved by ResolveVersion when empty
	BaseURL string // DefaultBaseURL when empty
	GoMod   string // go.mod to read the version from; "go.mod" when empty
	Force   bool   // download even when Dir already holds Version
	Client  *http.Client
}

// Result says what Fetch did.
type Result struct {
	Dir     string
	Version string
	Source  string // where the version came from: flag, go.mod or build info
	Fetched bool   // false when Dir already held Version
	Files   int
}

// releaseRe is a release tag: vMAJOR.MINOR.PATCH with an optional pre-release.
var releaseRe = regexp.MustCompile(`^v\d+\.\d+\.\d+(-[0-9A-Za-z.]+)?$`)

// pseudoRe is a Go pseudo-version's tail (-yyyymmddhhmmss-abcdefabcdef): a
// commit, not a release, so it has no web app asset.
var pseudoRe = regexp.MustCompile(`\d{14}-[0-9a-f]{12}$`)

// IsRelease reports whether v names a published release tag.
func IsRelease(v string) bool {
	return releaseRe.MatchString(v) && !pseudoRe.MatchString(v)
}

// ResolveVersion picks the Nabu release to fetch: the --version flag, else the
// version the course's go.mod requires, else the version this binary was built
// from. It returns the version and where it came from.
func ResolveVersion(flag, goMod string) (string, string, error) {
	if flag != "" {
		if !IsRelease(flag) {
			return "", "", fmt.Errorf("--version %q is not a release tag like v0.1.2", flag)
		}
		return flag, "flag", nil
	}
	if goMod == "" {
		goMod = "go.mod"
	}
	if v, err := versionFromGoMod(goMod); err != nil {
		return "", "", err
	} else if v != "" {
		if !IsRelease(v) {
			return "", "", fmt.Errorf("%s requires %s %s, which is not a release: no web app is published for it. "+
				"Require a tagged release (go get %s@latest) or pass --version", goMod, ModulePath, v, ModulePath)
		}
		return v, "go.mod", nil
	}
	if v := versionFromBuildInfo(); IsRelease(v) {
		return v, "build info", nil
	}
	return "", "", fmt.Errorf("cannot tell which Nabu release to fetch: no %s requiring %s here, "+
		"and this binary is not a release build. Pass --version vX.Y.Z", goMod, ModulePath)
}

// versionFromGoMod returns the version of ModulePath that goMod requires, ""
// when the file is missing or does not require it.
func versionFromGoMod(goMod string) (string, error) {
	data, err := os.ReadFile(goMod)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s: %w", goMod, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		line = strings.TrimPrefix(line, "require ")
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == ModulePath {
			return f[1], nil
		}
		if strings.HasPrefix(line, "module ") && strings.TrimSpace(strings.TrimPrefix(line, "module ")) == ModulePath {
			return "", nil // nabu's own repo: use build info
		}
	}
	return "", sc.Err()
}

// versionFromBuildInfo is Nabu's version in this binary: the dependency
// version in a course binary, the main module version in nabu itself (set by
// `go install …@vX.Y.Z`), or the release tag clog stamps into a release build.
func versionFromBuildInfo() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	if info.Main.Path == ModulePath {
		if IsRelease(info.Main.Version) {
			return info.Main.Version
		}
		if buildinfo.IsProductionBuild {
			return buildinfo.Info().Short // the release tag from clog's ldflags
		}
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == ModulePath {
			return dep.Version
		}
	}
	return ""
}

// AssetName is the web app archive of release version.
func AssetName(version string) string { return "nabu-web-" + version + ".tar.gz" }

// Fetch downloads, verifies and unpacks the web app. A folder that already
// holds the requested version is left alone (no network) unless Force is set.
func Fetch(ctx context.Context, o Options) (Result, error) {
	if o.Dir == "" {
		o.Dir = DefaultDir
	}
	if o.BaseURL == "" {
		o.BaseURL = DefaultBaseURL
	}
	if o.Client == nil {
		o.Client = &http.Client{Timeout: 2 * time.Minute}
	}
	v, src, err := ResolveVersion(o.Version, o.GoMod)
	if err != nil {
		return Result{}, err
	}
	res := Result{Dir: o.Dir, Version: v, Source: src}

	have, owned, err := installed(o.Dir)
	if err != nil {
		return res, err
	}
	if have == v && !o.Force {
		slog.Info("web app already present", "dir", o.Dir, "version", v)
		return res, nil
	}
	if !owned {
		return res, fmt.Errorf("%s exists and was not written by nabu web fetch (no %s): "+
			"choose another folder, or remove it first", o.Dir, MarkerFile)
	}

	base := strings.TrimRight(o.BaseURL, "/") + "/" + v + "/"
	asset := AssetName(v)
	sums, err := get(ctx, o.Client, base+"checksums.txt", 1<<20)
	if errors.Is(err, errNotFound) {
		return res, fmt.Errorf("no published release %s at %s (a tag with no release, or a typo): %w", v, o.BaseURL, err)
	}
	if err != nil {
		return res, err
	}
	want, err := checksumFor(sums, asset)
	if err != nil {
		return res, fmt.Errorf("release %s: %w", v, err)
	}
	archive, err := get(ctx, o.Client, base+asset, maxArchive)
	if err != nil {
		return res, err
	}
	sum := sha256.Sum256(archive)
	if got := hex.EncodeToString(sum[:]); got != want {
		return res, fmt.Errorf("%s: sha256 %s does not match checksums.txt (%s): refusing to unpack it", asset, got, want)
	}

	n, err := install(archive, o.Dir, v)
	if err != nil {
		return res, err
	}
	res.Fetched, res.Files = true, n
	slog.Info("web app fetched", "dir", o.Dir, "version", v, "from", src, "files", n, "sha256", want)
	return res, nil
}

// installed reads dir's marker: the version it holds, and whether the folder is
// ours to replace (missing, empty, or carrying the marker).
func installed(dir string) (version string, owned bool, err error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", true, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", dir, err)
	}
	if len(entries) == 0 {
		return "", true, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, MarkerFile))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read %s: %w", filepath.Join(dir, MarkerFile), err)
	}
	return strings.TrimSpace(string(data)), true, nil
}

// errNotFound is an HTTP 404 from the release host.
var errNotFound = errors.New("HTTP 404 not found")

// get downloads url, refusing a body larger than limit.
func get(ctx context.Context, c *http.Client, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only body
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("download %s: %w", url, errNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", url, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("download %s: larger than %d bytes", url, limit)
	}
	return data, nil
}

// checksumFor finds asset's sha256 in a checksums.txt (`<hex>  <name>` lines).
func checksumFor(sums []byte, asset string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == asset {
			if len(f[0]) != 64 {
				return "", fmt.Errorf("checksums.txt: malformed sha256 for %s", asset)
			}
			return strings.ToLower(f[0]), nil
		}
	}
	return "", fmt.Errorf("checksums.txt does not list %s: this release has no web app asset", asset)
}

// install unpacks archive into a fresh sibling of dir, writes the marker, then
// swaps it into place, so a failure never leaves dir half-written.
func install(archive []byte, dir, version string) (int, error) {
	parent := filepath.Dir(filepath.Clean(dir))
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return 0, fmt.Errorf("create %s: %w", parent, err)
	}
	tmp, err := os.MkdirTemp(parent, "."+filepath.Base(dir)+".new-")
	if err != nil {
		return 0, fmt.Errorf("unpack: %w", err)
	}
	defer os.RemoveAll(tmp) //nolint:errcheck // gone after the rename; cleanup on failure

	n, err := untar(archive, tmp)
	if err != nil {
		return 0, err
	}
	if _, err := os.Stat(filepath.Join(tmp, "index.html")); err != nil {
		return 0, errors.New("the archive has no index.html: not a Nabu web app")
	}
	if err := os.WriteFile(filepath.Join(tmp, MarkerFile), []byte(version+"\n"), 0o644); err != nil {
		return 0, fmt.Errorf("unpack: %w", err)
	}

	old := ""
	if _, err := os.Stat(dir); err == nil {
		old = tmp + ".old"
		if err := os.Rename(dir, old); err != nil {
			return 0, fmt.Errorf("replace %s: %w", dir, err)
		}
	}
	if err := os.Rename(tmp, dir); err != nil {
		if old != "" {
			_ = os.Rename(old, dir) // put the previous version back
		}
		return 0, fmt.Errorf("replace %s: %w", dir, err)
	}
	if old != "" {
		if err := os.RemoveAll(old); err != nil {
			slog.Warn("could not remove the previous web app", "dir", old, "err", err)
		}
	}
	return n, nil
}

// untar unpacks a gzipped tar of regular files and folders into dst. It
// refuses absolute paths, "..", links and devices: the archive is checked
// against checksums.txt, but unpacking stays safe even if it were not.
func untar(archive []byte, dst string) (int, error) {
	zr, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return 0, fmt.Errorf("unpack: %w", err)
	}
	tr := tar.NewReader(zr)
	files := 0
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return files, nil
		}
		if err != nil {
			return files, fmt.Errorf("unpack: %w", err)
		}
		name := path.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." || name == "" {
			continue
		}
		if !fs.ValidPath(name) {
			return files, fmt.Errorf("unpack: refusing path %q", h.Name)
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return files, fmt.Errorf("unpack: %w", err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return files, fmt.Errorf("unpack: %w", err)
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return files, fmt.Errorf("unpack: %w", err)
			}
			_, cerr := io.Copy(f, io.LimitReader(tr, maxArchive))
			if err := f.Close(); cerr == nil {
				cerr = err
			}
			if cerr != nil {
				return files, fmt.Errorf("unpack %s: %w", name, cerr)
			}
			files++
		default:
			return files, fmt.Errorf("unpack: refusing %q (not a regular file or folder)", h.Name)
		}
	}
}
