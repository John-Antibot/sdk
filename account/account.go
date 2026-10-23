// Package account is a client for the John Antibot account API: read the balance your key is charged against.
//
// A call to a paid endpoint takes its price from the balance when it starts and gets a 402 when the balance is less. Balance reads it before.
package account

import (
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
const DefaultEndpoint = "https://antibot.johnavenue.com/v1"

// defaultTimeout bounds one HTTP call.
const defaultTimeout = 20 * time.Second

// HTTPDoer is the transport for API calls; *http.Client satisfies it.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config configures the API connection.
type Config struct {
	// Endpoint is the API base URL (the one that ends in /v1); defaults to DefaultEndpoint.
	Endpoint string
	// APIKey is sent as the x-api-key header.
	APIKey string
	// Timeout is the deadline of one HTTP call; zero or negative means 20 seconds.
	Timeout time.Duration
	// HTTPClient is the API transport; defaults to a plain *http.Client.
	HTTPClient HTTPDoer
}

// Client sends API calls and is safe for concurrent use.
type Client struct {
	endpoint string
	apiKey   string
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
	return &Client{endpoint: endpoint, apiKey: config.APIKey, timeout: timeout, http: httpClient}, nil
}

// Endpoint returns the configured API base URL.
func (c *Client) Endpoint() string { return c.endpoint }

// Balance is what the account of a key has to spend.
type Balance struct {
	// Metered says the key is charged. A key that is not metered (one of the service's own) has no account and no balance: Micros is zero.
	Metered bool `json:"metered"`
	// Micros is the balance in millionths of a dollar, the unit of a price in the catalog (2000 is $0.002).
	Micros int64 `json:"balanceMicros"`
}

// Dollars is the balance in dollars, for display.
func (b *Balance) Dollars() float64 { return float64(b.Micros) / 1e6 }

// Balance reads the balance of the account of the key. It is free.
func (c *Client) Balance(ctx context.Context) (*Balance, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint+"/balance", nil)
	if err != nil {
		return nil, fmt.Errorf("account: build request: %w", err)
	}
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("account: request: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("account: read response: %w", err)
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
		return nil, &APIError{StatusCode: res.StatusCode, Message: message}
	}
	var balance Balance
	if err := json.Unmarshal(data, &balance); err != nil {
		return nil, fmt.Errorf("account: decode response: %w", err)
	}
	return &balance, nil
}

// APIError is a non-200 reply of the API; Message is its error text.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("account: HTTP %d: %s", e.StatusCode, e.Message)
}

// IsBadKey reports whether err is the API refusing the key: missing, unknown, revoked, or the key of a disabled account (HTTP 401).
func IsBadKey(err error) bool {
	var api *APIError
	return errors.As(err, &api) && api.StatusCode == http.StatusUnauthorized
}
