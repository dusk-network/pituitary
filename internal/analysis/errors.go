package analysis

import (
	"errors"
	"fmt"
)

// NotFoundError reports a missing indexed artifact.
type NotFoundError struct {
	Message string
}

func (e *NotFoundError) Error() string {
	return e.Message
}

// IsNotFound reports whether err wraps a not-found failure.
func IsNotFound(err error) bool {
	var target *NotFoundError
	return errors.As(err, &target)
}

func newSpecRefNotFoundError(ref string) *NotFoundError {
	return &NotFoundError{
		Message: fmt.Sprintf(
			"unknown --spec-ref %q: the ref is not present in the current index; run `pituitary search-specs --query ...` to inspect indexed spec refs or `pituitary index --rebuild` if the workspace changed",
			ref,
		),
	}
}

func newDocRefNotFoundError(ref string) *NotFoundError {
	return &NotFoundError{
		Message: fmt.Sprintf(
			"unknown --doc-ref %q: the ref is not present in the current index; run `pituitary preview-sources` to confirm the configured docs and `pituitary index --rebuild` if the workspace changed",
			ref,
		),
	}
}
