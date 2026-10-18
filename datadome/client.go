// Package datadome is a client for the DataDome payload generator (interstitial, slider, tags).
package datadome

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultEndpoint is the hosted API base used when Config.Endpoint is empty.
const DefaultEndpoint = "https://antibot.johnavenue.com/v1/datadome"

const defaultTimeout = 60 * time.Second

// HTTPDoer is the transport for generator calls; *http.Client satisfies it.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config configures the generator connection.
type Config struct {
	// Endpoint is the API base URL; defaults to DefaultEndpoint.
	Endpoint string
	// APIKey is sent as the x-api-key header when set.
	APIKey string
	// OrgKey is sent as the x-org-key header when set: software distributed under
	// an organization sends its key next to each user's own APIKey.
	OrgKey string
	// Timeout is the per-call deadline; zero or negative means 60 seconds.
	Timeout time.Duration
	// HTTPClient is the generator transport; defaults to a plain *http.Client.
	HTTPClient HTTPDoer
	// Defaults are merged into every Session the client creates.
	Defaults Session
}

// compressAbove is the request body size from which the SDK gzips it (about 2.5x smaller).
const compressAbove = 32 << 10

// Client sends generator requests and is safe for concurrent use.
type Client struct {
	ctx      context.Context
	endpoint string
	apiKey   string
	orgKey   string
	timeout  time.Duration
	http     HTTPDoer
	defaults Session
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
	return &Client{
		ctx: context.Background(), endpoint: endpoint, apiKey: config.APIKey, orgKey: config.OrgKey,
		timeout: timeout, http: httpClient, defaults: config.Defaults,
	}, nil
}

// WithContext returns a copy of the client that uses ctx for calls passed a nil context.
func (c *Client) WithContext(ctx context.Context) *Client {
	bound := *c
	bound.ctx = ctx
	return &bound
}

// Endpoint returns the configured generator base URL.
func (c *Client) Endpoint() string { return c.endpoint }

func (c *Client) post(ctx context.Context, route string, payload any, out any) error {
	if ctx == nil {
		ctx = c.ctx
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("datadome: marshal request: %w", err)
	}
	compressed := false
	if len(body) >= compressAbove { // interstitial/slider bodies carry the ~570 KB script
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if _, err := zw.Write(body); err == nil && zw.Close() == nil {
			body, compressed = buf.Bytes(), true
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+route, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("datadome: build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	if compressed {
		req.Header.Set("content-encoding", "gzip")
	}
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	if c.orgKey != "" {
		req.Header.Set("x-org-key", c.orgKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("datadome: request: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return fmt.Errorf("datadome: read response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
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
		return fmt.Errorf("datadome: decode response: %w", err)
	}
	return nil
}

func errNilInput(name string) error {
	return errors.New("datadome: " + name + " input is nil")
}

func nonEmpty(override, fallback string) string {
	if override != "" {
		return override
	}
	return fallback
}
