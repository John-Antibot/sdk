package cloudflare

import (
	"strings"
	"time"
)

// ChallengeInput is a page that serves a Cloudflare challenge.
type ChallengeInput struct {
	// URL is the page that serves the challenge. The site must be approved for your key.
	URL string
	// Proxy is the proxy the solve goes through, in any form a provider prints: http://user:pass@host:port, https://, socks5://, socks5h://,
	// host:port, host:port:user:pass or user:pass@host:port. It must have a public address. The clearance is for the address it exits from:
	// spend it from there.
	Proxy string
	// UserAgent is the user agent of your client, when you want the clearance for it: it must be the exact user agent of a Chrome build on macOS
	// that the API runs, and anything else is refused. Leave it empty to get ours, in Result.UserAgent, and send that one to the site.
	UserAgent string
	// Timezone is the IANA zone of the proxy's location, for example America/New_York: the page then reports the zone of the address.
	Timezone string
	// Session names the device: the same Session is the same device.
	Session string
	// Timeout is the job's wall, from 10 to 120 seconds; zero is 60.
	Timeout time.Duration
}

// TurnstileInput is a Turnstile widget on a page.
type TurnstileInput struct {
	// PageURL is the page with the widget. The site must be approved for your key.
	PageURL string
	// SiteKey is the site key of the widget.
	SiteKey string
	// Proxy, UserAgent, Timezone, Session and Timeout are as in ChallengeInput.
	Proxy     string
	UserAgent string
	Timezone  string
	Session   string
	Timeout   time.Duration
	// Action and CData are the widget's data-action and data-cdata.
	Action string
	CData  string
}

// Cookie is a cookie the solve ended with.
type Cookie struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Domain string `json:"domain,omitempty"`
	Path   string `json:"path,omitempty"`
}

// Result is what a finished job holds.
type Result struct {
	// Payload is "cf_clearance=<value>" for a challenge and the token for Turnstile.
	Payload string `json:"payload"`
	// UserAgent is the browser the clearance is for: send it in each request to the site.
	UserAgent string `json:"userAgent"`
	// Session is the device the solve ran as: send it as Session in the next solve for the same site session.
	Session string `json:"session,omitempty"`
	// Cookies are the cookies a browser on the page would send now (challenge): cf_clearance and the ones the solve learned.
	Cookies []Cookie `json:"cookies,omitempty"`
	// ExitIP is the address the site saw.
	ExitIP string `json:"exitIp,omitempty"`
	// Ms is how long the solve took.
	Ms int64 `json:"ms,omitempty"`
}

// Clearance is the value of the cf_clearance cookie of a challenge result ("" when there is none).
func (r *Result) Clearance() string {
	if r == nil {
		return ""
	}
	if v, ok := strings.CutPrefix(r.Payload, "cf_clearance="); ok {
		return v
	}
	for _, c := range r.Cookies {
		if c.Name == "cf_clearance" {
			return c.Value
		}
	}
	return ""
}

// Token is the token of a Turnstile result.
func (r *Result) Token() string {
	if r == nil {
		return ""
	}
	return r.Payload
}

// Job states.
const (
	StateQueued   = "queued"
	StateRunning  = "running"
	StateDone     = "done"
	StateFailed   = "failed"
	StateDeclined = "declined"
)

// Job is a solve as the API reports it.
type Job struct {
	ID     string  `json:"id"`
	State  string  `json:"state"`
	Task   string  `json:"task,omitempty"`
	Error  string  `json:"error,omitempty"`  // when failed or declined
	Retry  bool    `json:"retry,omitempty"`  // when declined: the site turned this attempt down; send a new job
	Result *Result `json:"result,omitempty"` // when done
}

// Terminal says the job has ended.
func (j *Job) Terminal() bool {
	return j.State != StateQueued && j.State != StateRunning
}

// outcome is the end of a terminal job: its Result, or why it has none.
func (j *Job) outcome() (*Result, error) {
	switch j.State {
	case StateDone:
		if j.Result == nil {
			return nil, &JobError{ID: j.ID, State: j.State, Message: "the job is done without a result"}
		}
		return j.Result, nil
	default:
		return nil, &JobError{ID: j.ID, State: j.State, Message: j.Error, Retry: j.Retry}
	}
}
