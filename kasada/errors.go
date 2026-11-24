package kasada

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is a non-2xx reply of the API; Message is its error text.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("kasada: HTTP %d: %s", e.StatusCode, e.Message)
}

func apiStatus(err error, status int) bool {
	var api *APIError
	return errors.As(err, &api) && api.StatusCode == status
}

func IsInsufficientBalance(err error) bool { return apiStatus(err, http.StatusPaymentRequired) }
func IsSiteNotApproved(err error) bool     { return apiStatus(err, http.StatusForbidden) }
func IsRateLimited(err error) bool         { return apiStatus(err, http.StatusTooManyRequests) }
func IsBadRequest(err error) bool          { return apiStatus(err, http.StatusBadRequest) }
func IsNotAvailable(err error) bool        { return apiStatus(err, http.StatusNotImplemented) }
