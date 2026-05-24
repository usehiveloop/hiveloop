package nango

import (
	"errors"
	"fmt"
)

var ErrNotFound = errors.New("nango resource not found")

type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("nango API error %d: %s", e.StatusCode, e.Body)
}

func (e *APIError) Is(target error) bool {
	return target == ErrNotFound && e.StatusCode == 404
}
