package pathselector

import (
	"fmt"
	pathpkg "path"
	"path/filepath"
	"strings"
)

// Pattern is a compiled filesystem source selector.
type Pattern struct {
	raw       string
	parts     []string
	recursive bool
}

// Compile validates and compiles a slash-separated source-relative selector.
func Compile(pattern string) (Pattern, error) {
	normalized := filepath.ToSlash(strings.TrimSpace(pattern))
	if normalized == "" {
		return Pattern{}, fmt.Errorf("value must not be empty")
	}
	if filepath.IsAbs(normalized) || pathpkg.IsAbs(normalized) {
		return Pattern{}, fmt.Errorf("must be relative to the source root")
	}

	parts := strings.Split(normalized, "/")
	recursive := false
	for _, part := range parts {
		switch part {
		case "":
			return Pattern{}, fmt.Errorf("must not contain empty path segments")
		case ".", "..":
			return Pattern{}, fmt.Errorf("must not contain %q path segments", part)
		case "**":
			recursive = true
			continue
		}
		if _, err := pathpkg.Match(part, "placeholder"); err != nil {
			return Pattern{}, err
		}
	}
	return Pattern{raw: normalized, parts: parts, recursive: recursive}, nil
}

// Validate reports whether pattern can be compiled as a source selector.
func Validate(pattern string) error {
	_, err := Compile(pattern)
	return err
}

// Match reports whether relPath matches pattern.
func Match(pattern, relPath string) (bool, error) {
	compiled, err := Compile(pattern)
	if err != nil {
		return false, err
	}
	return compiled.Match(relPath)
}

// Match reports whether relPath matches the compiled selector.
func (p Pattern) Match(relPath string) (bool, error) {
	relPath = filepath.ToSlash(strings.TrimSpace(relPath))
	if !p.recursive {
		return pathpkg.Match(p.raw, relPath)
	}
	pathParts, err := splitRelativePath(relPath)
	if err != nil {
		return false, err
	}
	return matchParts(p.parts, pathParts)
}

func splitRelativePath(value string) ([]string, error) {
	if value == "" {
		return nil, fmt.Errorf("value must not be empty")
	}
	parts := strings.Split(value, "/")
	for _, part := range parts {
		if part == "" {
			return nil, fmt.Errorf("must not contain empty path segments")
		}
		if part == "." || part == ".." {
			return nil, fmt.Errorf("must not contain %q path segments", part)
		}
	}
	return parts, nil
}

func matchParts(patternParts, pathParts []string) (bool, error) {
	type matchState struct {
		patternIndex int
		pathIndex    int
	}
	memo := make(map[matchState]bool)
	var match func(patternIndex, pathIndex int) (bool, error)
	match = func(patternIndex, pathIndex int) (bool, error) {
		state := matchState{patternIndex: patternIndex, pathIndex: pathIndex}
		if failed, ok := memo[state]; ok && failed {
			return false, nil
		}
		if patternIndex == len(patternParts) {
			return pathIndex == len(pathParts), nil
		}
		if patternParts[patternIndex] == "**" {
			for nextPathIndex := pathIndex; nextPathIndex <= len(pathParts); nextPathIndex++ {
				if ok, err := match(patternIndex+1, nextPathIndex); ok || err != nil {
					return ok, err
				}
			}
			memo[state] = true
			return false, nil
		}
		if pathIndex == len(pathParts) {
			memo[state] = true
			return false, nil
		}
		ok, err := pathpkg.Match(patternParts[patternIndex], pathParts[pathIndex])
		if err != nil || !ok {
			if err == nil {
				memo[state] = true
			}
			return ok, err
		}
		ok, err = match(patternIndex+1, pathIndex+1)
		if err == nil && !ok {
			memo[state] = true
		}
		return ok, err
	}
	return match(0, 0)
}
