// Package recaptcha is a client for the John Antibot reCAPTCHA API: a reCAPTCHA v3 token for a page, classic or Enterprise, made through your own
// proxy.
//
// A solve is a job. You send the page, the site key and the proxy, the API answers at once with a job id, and the job runs on our side: the page,
// the reCAPTCHA client of the site and every request of the solve go through your proxy, so the token is for the address that proxy exits from.
// SolveV3 and SolveEnterprise send a job and wait for it; SubmitV3, SubmitEnterprise, Job and Wait are the same in steps. Send the token to the
// site from the same address, with Result.Headers.
package recaptcha

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
const DefaultEndpoint = "https://antibot.johnavenue.com/v1/recaptcha"

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

// SolveV3 makes a token for a page that loads the classic reCAPTCHA v3 script (api.js) and waits for it.
func (c *Client) SolveV3(ctx context.Context, in *Input) (*Result, error) {
	job, err := c.SubmitV3(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.Wait(ctx, job)
}

// SolveEnterprise makes a token for a page that loads reCAPTCHA Enterprise (enterprise.js) and waits for it.
func (c *Client) SolveEnterprise(ctx context.Context, in *Input) (*Result, error) {
	job, err := c.SubmitEnterprise(ctx, in)
	if err != nil {
		return nil, err
	}
	return c.Wait(ctx, job)
}

// SubmitV3 sends a classic v3 job and returns it queued: its ID is what Job and Wait take. The price is charged now and returned if the job fails.
func (c *Client) SubmitV3(ctx context.Context, in *Input) (*Job, error) {
	return c.submit(ctx, "/v3", in)
}

// SubmitEnterprise sends an Enterprise job and returns it queued.
func (c *Client) SubmitEnterprise(ctx context.Context, in *Input) (*Job, error) {
	return c.submit(ctx, "/v3-enterprise", in)
}

// Job reads a job. With wait above zero the call returns as soon as the job has ended, or after wait (up to 25 seconds) with the state it has.
func (c *Client) Job(ctx context.Context, id string, wait time.Duration) (*Job, error) {
	if id == "" {
		return nil, errors.New("recaptcha: a job id is required")
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

// Wait follows a job until it ends and returns its Result. A job that failed or was declined is a *JobError; ctx ends the wait, not the job.
func (c *Client) Wait(ctx context.Context, job *Job) (*Result, error) {
	if job == nil {
		return nil, errors.New("recaptcha: job is nil")
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

func (c *Client) submit(ctx context.Context, route string, in *Input) (*Job, error) {
	if in == nil {
		return nil, errors.New("recaptcha: input is nil")
	}
	if in.URL == "" || in.SiteKey == "" || in.Proxy == "" {
		return nil, errors.New("recaptcha: a solve needs a URL, a site key and a proxy")
	}
	payload := map[string]any{"url": in.URL, "siteKey": in.SiteKey, "proxy": in.Proxy}
	if in.Action != "" {
		payload["action"] = in.Action
	}
	if in.Timeout > 0 {
		payload["timeoutMs"] = in.Timeout.Milliseconds()
	}
	var job Job
	if err := c.do(ctx, http.MethodPost, route, payload, &job); err != nil {
		return nil, err
	}
	if job.ID == "" {
		return nil, errors.New("recaptcha: the API answered without a job id")
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
			return fmt.Errorf("recaptcha: marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.endpoint+route, body)
	if err != nil {
		return fmt.Errorf("recaptcha: build request: %w", err)
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
		return fmt.Errorf("recaptcha: request: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("recaptcha: read response: %w", err)
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
		return fmt.Errorf("recaptcha: decode response: %w", err)
	}
	return nil
}

// retryable is a poll that failed for a reason that may not be there next time: the network, or a server that had a bad moment.
func retryable(err error) bool {
	var api *APIError
	if errors.As(err, &api) {
		return api.StatusCode >= 500
	}
	return true
}
