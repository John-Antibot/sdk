// Package cloudflare is a client for the John Antibot Cloudflare API: clear a challenge page or solve a Turnstile widget through your own proxy.
//
// A solve is a job. You send the page and the proxy, the API answers at once with a job id, and the job runs on our side: every request of
// the solve goes through your proxy, so the clearance is for the address that proxy exits from. SolveChallenge and SolveTurnstile send a job and
// wait for it; SubmitChallenge, SubmitTurnstile, Job and Wait are the same in steps. Spend the clearance from the same address, with
// Result.UserAgent.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// DefaultEndpoint is the hosted API base used when Config.Endpoint is empty.
const DefaultEndpoint = "https://antibot.johnavenue.com/v1/cloudflare"

// defaultTimeout bounds one HTTP call. A poll waits up to pollWait on the server, so a call needs more than that.
const defaultTimeout = 40 * time.Second

// pollWait is how long a poll asks the server to hold it for a job that has not ended (the server's most is 25 seconds).
const pollWait = 25 * time.Second

// pollRetries is how many polls in a row may fail (the network, a 5xx) before Wait gives the job up. The job runs on, and Job can ask again.
const pollRetries = 3

// HTTPDoer is the transport for API calls; *http.Client satisfies it.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config configures the API connection.
type Config struct {
	// Endpoint is the API base URL; defaults to DefaultEndpoint.
	Endpoint string
	// APIKey is sent as the x-api-key header when set.
	APIKey string
	// OrgKey is sent as the x-org-key header when set: software distributed under an organization sends its key next to each user's own APIKey.
	OrgKey string
	// Timeout is the deadline of one HTTP call (not of a solve: pass a context with a deadline for that); zero or negative means 40 seconds.
	Timeout time.Duration
	// HTTPClient is the API transport; defaults to a plain *http.Client. It carries your calls to the API, not the solve: the proxy you
	// send in each input is the solve's.
	HTTPClient HTTPDoer
}

// Client sends API calls and is safe for concurrent use.
type Client struct {
	endpoint string
	apiKey   string
	orgKey   string
	timeout  time.Duration
	http     HTTPDoer
}

// New builds a Client.
func New(config Config) (*Client, error) {
	endpoint := strings.TrimRight(config.Endpoint, "/")
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Transport: &http.Transport{}}
	}
	return &Client{endpoint: endpoint, apiKey: config.APIKey, orgKey: config.OrgKey, timeout: timeout, http: httpClient}, nil
}

// Endpoint returns the configured API base URL.
func (c *Client) Endpoint() string { return c.endpoint }

// SolveChallenge clears the challenge page at in.URL and waits for the clearance.
func (c *Client) SolveChallenge(ctx context.Context, in *ChallengeInput) (*Result, error) {
	job, err := c.SubmitChallenge(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.Wait(ctx, job)
}

// SolveTurnstile solves the Turnstile widget on in.PageURL and waits for the token.
func (c *Client) SolveTurnstile(ctx context.Context, in *TurnstileInput) (*Result, error) {
	job, err := c.SubmitTurnstile(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.Wait(ctx, job)
}

// SubmitChallenge sends a challenge job and returns it queued: its ID is what Job and Wait take. The price is charged now and returned if the job
// fails or is declined.
func (c *Client) SubmitChallenge(ctx context.Context, in *ChallengeInput) (*Job, error) {
	if in == nil {
		return nil, errors.New("cloudflare: challenge input is nil")
	}
	if in.URL == "" || in.Proxy == "" {
		return nil, errors.New("cloudflare: a challenge needs a URL and a proxy")
	}
	return c.submit(ctx, "/challenge", map[string]any{
		"url": in.URL, "proxy": in.Proxy, "userAgent": in.UserAgent, "timezone": in.Timezone, "session": in.Session, "timeoutMs": millis(in.Timeout),
	})
}

// SubmitTurnstile sends a Turnstile job and returns it queued.
func (c *Client) SubmitTurnstile(ctx context.Context, in *TurnstileInput) (*Job, error) {
	if in == nil {
		return nil, errors.New("cloudflare: turnstile input is nil")
	}
	if in.PageURL == "" || in.SiteKey == "" || in.Proxy == "" {
		return nil, errors.New("cloudflare: a turnstile solve needs a page URL, a site key and a proxy")
	}
	return c.submit(ctx, "/turnstile", map[string]any{
		"pageUrl": in.PageURL, "sitekey": in.SiteKey, "proxy": in.Proxy, "action": in.Action, "cData": in.CData,
		"userAgent": in.UserAgent, "timezone": in.Timezone, "session": in.Session, "timeoutMs": millis(in.Timeout),
	})
}

// Job reads a job. With wait above zero the call returns as soon as the job has ended, or after wait (up to 25 seconds) with the state it has.
func (c *Client) Job(ctx context.Context, id string, wait time.Duration) (*Job, error) {
	if id == "" {
		return nil, errors.New("cloudflare: a job id is required")
	}
	path := "/jobs/" + url.PathEscape(id)
	if wait > 0 {
		path += "?wait=" + strconv.Itoa(int((wait+time.Second-1)/time.Second))
	}
	var job Job
	if err := c.do(ctx, http.MethodGet, path, nil, &job); err != nil {
		return nil, err
	}
	return &job, nil
}

// Wait follows a job until it ends and returns its Result. A job that failed or was declined is a *JobError (IsDeclined); ctx ends the wait, not
// the job.
func (c *Client) Wait(ctx context.Context, job *Job) (*Result, error) {
	if job == nil {
		return nil, errors.New("cloudflare: job is nil")
	}
	failures := 0
	for {
		if job.Terminal() {
			return job.outcome()
		}
		next, err := c.Job(ctx, job.ID, pollWait)
		switch {
		case err == nil:
			job, failures = next, 0
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case retryable(err) && failures < pollRetries:
			failures++
			select {
			case <-time.After(time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		default:
			return nil, err
		}
	}
}

func (c *Client) submit(ctx context.Context, route string, payload map[string]any) (*Job, error) {
	for k, v := range payload { // an empty optional is left out
		switch x := v.(type) {
		case string:
			if x == "" {
				delete(payload, k)
			}
		case int64:
			if x == 0 {
				delete(payload, k)
			}
		}
	}
	var job Job
	if err := c.do(ctx, http.MethodPost, route, payload, &job); err != nil {
		return nil, err
	}
	if job.ID == "" {
		return nil, errors.New("cloudflare: the API answered without a job id")
	}
	return &job, nil
}

func (c *Client) do(ctx context.Context, method, route string, payload any, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("cloudflare: marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+route, body)
	if err != nil {
		return fmt.Errorf("cloudflare: build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	if c.orgKey != "" {
		req.Header.Set("x-org-key", c.orgKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cloudflare: request: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("cloudflare: read response: %w", err)
	}
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &failure)
		message := failure.Error
		if message == "" {
			message = strings.TrimSpace(string(data))
		}
		if message == "" {
			message = res.Status
		}
		return &APIError{StatusCode: res.StatusCode, Message: message}
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("cloudflare: decode response: %w", err)
	}
	return nil
}

// millis is a job's wall in the API's unit (zero: the API's default).
func millis(d time.Duration) int64 { return d.Milliseconds() }

// retryable is a poll that failed for a reason that may not be there next time: the network, or a server that had a bad moment.
func retryable(err error) bool {
	var api *APIError
	if errors.As(err, &api) {
		return api.StatusCode >= 500
	}
	return true
}
