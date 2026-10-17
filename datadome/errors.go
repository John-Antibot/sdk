package datadome

import (
	"errors"
	"fmt"
	"strings"
)

// APIError is a non-200 generator reply; Message is the service's error text.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("datadome: HTTP %d: %s", e.StatusCode, e.Message)
}

// IsHardblock reports whether err is the generator refusing an unsolvable hardblock page.
func IsHardblock(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && strings.Contains(apiErr.Message, "hardblock")
}

// IsPuzzleUnsupported reports whether err is the generator refusing a puzzle captcha page.
func IsPuzzleUnsupported(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && strings.Contains(apiErr.Message, "puzzle slider unsupported")
}

// IsRateLimited reports whether err is an HTTP 429 from the gateway.
func IsRateLimited(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == 429
}
