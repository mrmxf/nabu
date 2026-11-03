// Package assetpipe publishes the Hugo-style assets tree to the served public
// directory: every file under the assets dir maps to the same path under
// pb_public with the assets/ prefix stripped, and YAML data files are converted
// to pretty JSON. So assets/data/blobby.yaml is served at /data/blobby.json —
// no path surprises, and a teacher can read the JSON in the browser.
package assetpipe

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mrmxf/nabu/internal/source"
)

// Build walks the assets tree src and publishes it under dstDir (e.g.
// pb_public): *.yaml/*.yml become pretty *.json; everything else is copied as-is.
// A missing tree is not an error (assets are optional).
func Build(src source.Tree, dstDir string) error {
	if !src.Exists() {
		slog.Debug("assetpipe: no assets tree; skipping", "source", src.Label)
		return nil
	}
	count := 0
	err := fs.WalkDir(src.FS, ".", func(rel string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := path.Ext(rel)
		switch strings.ToLower(ext) {
		case ".yaml", ".yml":
			dst := filepath.Join(dstDir, filepath.FromSlash(strings.TrimSuffix(rel, ext)+".json"))
			if err := convertYAML(src, rel, dst); err != nil {
				return err
			}
		default:
			if err := src.CopyTo(rel, filepath.Join(dstDir, filepath.FromSlash(rel))); err != nil {
				return err
			}
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	slog.Info("assetpipe built", "source", src.Label, "dst", dstDir, "files", count)
	return nil
}

// convertYAML parses the YAML file rel in src and writes it to dst as indented
// JSON (yaml.v3 decodes mappings to map[string]any, so encoding/json handles the
// result directly).
func convertYAML(src source.Tree, rel, dst string) error {
	raw, err := fs.ReadFile(src.FS, rel)
	if err != nil {
		return fmt.Errorf("assetpipe read %s: %w", src.Where(rel), err)
	}
	var v any
	if err := yaml.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("assetpipe parse %s: %w", src.Where(rel), err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("assetpipe marshal %s: %w", src.Where(rel), err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, append(out, '\n'), 0o644)
}
