package cloudflare

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// APIError is a non-2xx reply of the API; Message is its error text.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("cloudflare: HTTP %d: %s", e.StatusCode, e.Message)
}

// JobError is a job that ended without a result: failed (the price is returned) or declined (the site turned the attempt down: not charged, a
// new job is a new attempt).
type JobError struct {
	ID      string
	State   string // failed or declined
	Message string
	Retry   bool
}

func (e *JobError) Error() string {
	return fmt.Sprintf("cloudflare: job %s %s: %s", e.ID, e.State, e.Message)
}

// IsDeclined reports whether err is a job the site declined: send another.
func IsDeclined(err error) bool {
	var job *JobError
	return errors.As(err, &job) && job.State == StateDeclined
}

// IsNoChallenge reports whether err is a page that served no challenge (there is nothing to clear).
func IsNoChallenge(err error) bool {
	var job *JobError
	return errors.As(err, &job) && strings.Contains(job.Message, "serves no challenge")
}

func apiStatus(err error, status int) bool {
	var api *APIError
	return errors.As(err, &api) && api.StatusCode == status
}

// IsInsufficientBalance reports whether err is the API refusing a job for the account's balance (HTTP 402).
func IsInsufficientBalance(err error) bool { return apiStatus(err, http.StatusPaymentRequired) }

// IsSiteNotApproved reports whether err is the API refusing a site that is not approved for the key (HTTP 403).
func IsSiteNotApproved(err error) bool { return apiStatus(err, http.StatusForbidden) }

// IsRateLimited reports whether err is the API holding the account at its open jobs (HTTP 429): wait for one to finish.
func IsRateLimited(err error) bool { return apiStatus(err, http.StatusTooManyRequests) }

// IsBadRequest reports whether err is the API refusing the input (HTTP 400): the proxy, the user agent, a field.
func IsBadRequest(err error) bool { return apiStatus(err, http.StatusBadRequest) }

// IsRotatingProxy reports whether err is a job refused because the proxy exits from a different address on each connection: the clearance
// is for one address and the page is asked for from another. Use a sticky session of the provider (its session id in the username) or a
// static proxy. The price is returned.
func IsRotatingProxy(err error) bool {
	var job *JobError
	return errors.As(err, &job) && strings.Contains(job.Message, "different address on each connection")
}
