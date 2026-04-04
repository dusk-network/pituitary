package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type plannedEdit struct {
	Replace   string
	With      string
	Line      int
	StartByte int
	EndByte   int
	start     int
	end       int
}

func uniqueMatch(text, needle string) (int, int, string, bool) {
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return 0, 0, "", false
	}
	indices := allMatchIndicesFold(text, needle)
	if len(indices) != 1 {
		return 0, 0, "", false
	}
	start := indices[0]
	end := start + len(needle)
	return start, end, text[start:end], true
}

func allMatchIndicesFold(text, needle string) []int {
	lowerText := strings.ToLower(text)
	lowerNeedle := strings.ToLower(needle)
	var result []int
	offset := 0
	for {
		index := strings.Index(lowerText[offset:], lowerNeedle)
		if index < 0 {
			break
		}
		start := offset + index
		result = append(result, start)
		offset = start + len(lowerNeedle)
	}
	return result
}

func contentChecksum(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

func resolveSourceFilePath(workspaceRoot, sourceRef string) (string, error) {
	relative := strings.TrimSpace(strings.TrimPrefix(sourceRef, "file://"))
	if relative == "" {
		return "", fmt.Errorf("source_ref is empty")
	}
	return filepath.Join(workspaceRoot, filepath.FromSlash(relative)), nil
}

func applyEdits(path, expectedContent, expectedChecksum string, edits []plannedEdit) error {
	// #nosec G304 -- path is the previously planned workspace file being updated in place.
	currentBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s before apply: %w", path, err)
	}
	current := string(currentBytes)
	if contentChecksum(current) != expectedChecksum || current != expectedContent {
		return fmt.Errorf("%s changed since fix planning; rerun `pituitary fix` against a fresh index", filepath.ToSlash(path))
	}

	updated := current
	for i := len(edits) - 1; i >= 0; i-- {
		edit := edits[i]
		updated = updated[:edit.start] + edit.With + updated[edit.end:]
	}

	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s before apply: %w", path, err)
	}
	tempFile, err := os.CreateTemp(filepath.Dir(path), ".pituitary-fix-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.WriteString(updated); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tempFile.Chmod(info.Mode()); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
