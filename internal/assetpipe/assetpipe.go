// Package assetpipe publishes the Hugo-style assets tree to the served public
// directory: every file under the assets dir maps to the same path under
// pb_public with the assets/ prefix stripped, and YAML data files are converted
// to pretty JSON. So assets/data/blobby.yaml is served at /data/blobby.json —
// no path surprises, and a teacher can read the JSON in the browser.
package assetpipe

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Build walks srcDir (e.g. "assets") and publishes it under dstDir (e.g.
// pb_public): *.yaml/*.yml become pretty *.json; everything else is copied as-is.
// A missing srcDir is not an error (assets are optional).
func Build(srcDir, dstDir string) error {
	info, err := os.Stat(srcDir)
	if err != nil || !info.IsDir() {
		slog.Debug("assetpipe: no assets dir; skipping", "dir", srcDir)
		return nil
	}
	count := 0
	err = filepath.WalkDir(srcDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".yaml", ".yml":
			dst := filepath.Join(dstDir, strings.TrimSuffix(rel, filepath.Ext(rel))+".json")
			if err := convertYAML(path, dst); err != nil {
				return err
			}
		default:
			if err := copyFile(path, filepath.Join(dstDir, rel)); err != nil {
				return err
			}
		}
		count++
		return nil
	})
	if err != nil {
		return err
	}
	slog.Info("assetpipe built", "src", srcDir, "dst", dstDir, "files", count)
	return nil
}

// convertYAML parses a YAML file and writes it as indented JSON (yaml.v3 decodes
// mappings to map[string]any, so encoding/json handles the result directly).
func convertYAML(src, dst string) error {
	raw, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	var v any
	if err := yaml.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("assetpipe parse %s: %w", src, err)
	}
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("assetpipe marshal %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, append(out, '\n'), 0o644)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
