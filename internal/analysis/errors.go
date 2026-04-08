package analysis

import (
	"errors"
	"fmt"
	"strings"
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

func newSpecRefNotFoundError(ref string, availableRefs []string) *NotFoundError {
	if len(availableRefs) == 0 {
		return &NotFoundError{
			Message: fmt.Sprintf(
				"unknown --spec-ref %q: the ref is not present in the current index; run `pituitary search-specs --query ...` to inspect indexed spec refs or `pituitary index --rebuild` if the workspace changed",
				ref,
			),
		}
	}

	preview := append([]string(nil), availableRefs...)
	extra := 0
	if len(preview) > 12 {
		extra = len(preview) - 12
		preview = preview[:12]
	}

	message := fmt.Sprintf(
		"unknown --spec-ref %q: the ref is not present in the current index; available spec refs: %s",
		ref,
		strings.Join(preview, ", "),
	)
	if extra > 0 {
		message += fmt.Sprintf(" (+%d more; run `pituitary search-specs --query ...` to inspect the full index)", extra)
	}
	message += "; run `pituitary index --rebuild` if the workspace changed"

	return &NotFoundError{Message: message}
}

func newDocRefNotFoundError(ref string) *NotFoundError {
	return &NotFoundError{
		Message: fmt.Sprintf(
			"unknown --doc-ref %q: the ref is not present in the current index; run `pituitary preview-sources` to confirm the configured docs and `pituitary index --rebuild` if the workspace changed",
			ref,
		),
	}
}
