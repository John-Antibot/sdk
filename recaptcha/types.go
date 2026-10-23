package recaptcha

import (
	"net/http"
	"time"
)

// Input is a page that uses reCAPTCHA v3, and the proxy the token is made through.
type Input struct {
	// URL is the page the token is for: the page whose script asks for it. The site must be approved for your key.
	URL string
	// SiteKey is the site's reCAPTCHA key (the render= value of the script, or the key the page passes to execute).
	SiteKey string
	// Action is the action the page passes to execute, for example "login". Leave it empty for "submit".
	Action string
	// Proxy is the proxy the solve goes through, in any form a provider prints: http://user:pass@host:port, https://, socks5://, socks5h://,
	// host:port, host:port:user:pass or user:pass@host:port. It must have a public address. The token is made for the address it exits from:
	// send it to the site from there.
	Proxy string
	// Timeout is the job's wall, from 10 seconds to the most the API allows (60 on the hosted API); zero is the API's default (30).
	Timeout time.Duration
}

// Result is what a finished job holds: the token, and the browser it was made as.
type Result struct {
	// Token is the reCAPTCHA token: the value of the g-recaptcha-response field, or what the page's execute call would have returned.
	Token string `json:"token"`
	// ExpiresAt is when Google stops accepting the token, counted from the start of the job. It is the zero time when the API gave none.
	ExpiresAt time.Time `json:"expiresAt"`
	// UserAgent and AcceptLanguage are the browser the token was made as: send them in the request that carries the token.
	UserAgent      string `json:"userAgent"`
	AcceptLanguage string `json:"acceptLanguage"`
	// SecUa, SecUaMobile and SecUaPlatform are the client hint headers of that browser (sec-ch-ua, sec-ch-ua-mobile, sec-ch-ua-platform).
	SecUa         string `json:"secUa,omitempty"`
	SecUaMobile   string `json:"secUaMobile,omitempty"`
	SecUaPlatform string `json:"secUaPlatform,omitempty"`
}

// Headers are the headers that say who made the token (user-agent, accept-language and the client hints), for the request that carries it. A
// header the result has no value for is left out.
func (r *Result) Headers() http.Header {
	h := http.Header{}
	if r == nil {
		return h
	}
	for name, value := range map[string]string{
		"user-agent": r.UserAgent, "accept-language": r.AcceptLanguage,
		"sec-ch-ua": r.SecUa, "sec-ch-ua-mobile": r.SecUaMobile, "sec-ch-ua-platform": r.SecUaPlatform,
	} {
		if value != "" {
			h.Set(name, value)
		}
	}
	return h
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
	ID            string  `json:"id"`
	State         string  `json:"state"`
	Task          string  `json:"task,omitempty"`
	Error         string  `json:"error,omitempty"`         // when failed or declined
	Retry         bool    `json:"retry,omitempty"`         // when declined: the attempt was turned down; send a new job
	Result        *Result `json:"result,omitempty"`        // when done
	ResultExpired bool    `json:"resultExpired,omitempty"` // when done: the API no longer holds the result
}

// Terminal says the job has ended.
func (j *Job) Terminal() bool {
	return j.State != StateQueued && j.State != StateRunning
}

// outcome is the end of a terminal job: its Result, or why it has none.
func (j *Job) outcome() (*Result, error) {
	if j.State != StateDone {
		return nil, &JobError{ID: j.ID, State: j.State, Message: j.Error, Retry: j.Retry}
	}
	switch {
	case j.Result != nil:
		return j.Result, nil
	case j.ResultExpired:
		return nil, &JobError{ID: j.ID, State: j.State, Message: "the job is done but the API no longer holds its result"}
	default:
		return nil, &JobError{ID: j.ID, State: j.State, Message: "the job is done without a result"}
	}
}
