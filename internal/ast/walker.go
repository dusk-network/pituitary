package ast

import (
	"os"
	"path/filepath"
	"strings"
)

// ignoredDirs are directory names skipped during workspace walking.
var ignoredDirs = map[string]bool{
	".git":         true,
	".hg":          true,
	".svn":         true,
	"node_modules": true,
	"vendor":       true,
	"__pycache__":  true,
	".pituitary":   true,
	"target":       true,
	"dist":         true,
	"build":        true,
	".next":        true,
}

// MaxWalkFiles is the maximum number of source files returned by WalkWorkspace.
// This bounds memory and parse time for very large workspaces.
const MaxWalkFiles = 5000

// WalkWorkspace returns workspace-relative paths for all source files in
// supported languages, skipping common ignored directories. At most
// MaxWalkFiles paths are returned.
func WalkWorkspace(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			name := d.Name()
			if ignoredDirs[name] || (strings.HasPrefix(name, ".") && name != ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if len(paths) >= MaxWalkFiles {
			return filepath.SkipAll
		}
		lang := DetectLanguage(path)
		if lang == "" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	return paths, err
}
