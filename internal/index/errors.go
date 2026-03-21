package index

import (
	"errors"
	"fmt"
)

// MissingIndexError reports that the configured query index has not been built yet.
type MissingIndexError struct {
	Path string
}

func (e *MissingIndexError) Error() string {
	return fmt.Sprintf("index %s does not exist; run `pituitary index --rebuild`", e.Path)
}

// IsMissingIndex reports whether err wraps a missing-index failure.
func IsMissingIndex(err error) bool {
	var target *MissingIndexError
	return errors.As(err, &target)
}

// MissingIndexPath returns the configured index path for a missing-index failure.
func MissingIndexPath(err error) string {
	var target *MissingIndexError
	if errors.As(err, &target) {
		return target.Path
	}
	return ""
}
