package analysis

import "errors"

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
