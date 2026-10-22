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
	// Hardblock: the site blocked the session outright (DataDome's hardblock page, no slider to solve). The gateway answers 422 and
	// charges nothing; change the proxy and the session.
	Hardblock bool
}

func (e *APIError) Error() string {
	return fmt.Sprintf("datadome: HTTP %d: %s", e.StatusCode, e.Message)
}

// IsHardblock reports whether err is the generator refusing an unsolvable hardblock page (a 422 with "hardblock": true; an earlier
// gateway answered 500 and said so in the message).
func IsHardblock(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.Hardblock || strings.Contains(apiErr.Message, "hardblock"))
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
