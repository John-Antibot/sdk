// Package kasada is a client for the John Antibot Kasada payload API.
//
// It is stateless payload generation. The caller fetches the block or
// /fp page and its ips.js through its own client and proxy, sends that
// script here, and posts the returned binary payload and x-kpsdk-*
// headers to /tl itself from the IP it named.
package kasada

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
const DefaultEndpoint = "https://antibot.johnavenue.com/v1/kasada"

const defaultTimeout = 60 * time.Second
const compressAbove = 32 << 10

// HTTPDoer is the transport for API calls; *http.Client satisfies it.
type HTTPDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Config configures the API connection.
type Config struct {
	Endpoint   string
	APIKey     string
	OrgKey     string
	Timeout    time.Duration
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

// Version reads the x-kpsdk-v value from p.js by running it as a
// collected device and reading the header it stamps on /mfc.
func (c *Client) Version(ctx context.Context, in *VersionInput) (*VersionResult, error) {
	if in == nil {
		return nil, errors.New("kasada: input is nil")
	}
	if in.Script == "" || in.PageURL == "" {
		return nil, errors.New("kasada: a version needs script and pageUrl")
	}
	payload := map[string]any{"script": in.Script, "pageUrl": in.PageURL}
	if in.UserAgent != "" {
		payload["userAgent"] = in.UserAgent
	}
	if in.AcceptLanguage != "" {
		payload["acceptLanguage"] = in.AcceptLanguage
	}
	if in.Session != "" {
		payload["session"] = in.Session
	}
	var out VersionResult
	if err := c.post(ctx, "/version", payload, &out); err != nil {
		return nil, err
	}
	if out.Version == "" {
		return nil, errors.New("kasada: the API answered without a version")
	}
	return &out, nil
}

// GeneratePayload runs ips.js as a collected device and returns the
// binary /tl body and headers, ready to post unchanged.
func (c *Client) GeneratePayload(ctx context.Context, in *PayloadInput) (*PayloadResult, error) {
	if in == nil {
		return nil, errors.New("kasada: input is nil")
	}
	if in.Script == "" || in.PageURL == "" || in.UserAgent == "" || in.IP == "" || in.AcceptLanguage == "" {
		return nil, errors.New("kasada: a payload needs script, pageUrl, userAgent, ip and acceptLanguage")
	}
	if in.HTML == "" && in.IpsLink == "" {
		return nil, errors.New("kasada: a payload needs html or ipsLink")
	}
	payload := map[string]any{
		"script": in.Script, "pageUrl": in.PageURL, "userAgent": in.UserAgent,
		"ip": in.IP, "acceptLanguage": in.AcceptLanguage,
	}
	if in.HTML != "" {
		payload["html"] = in.HTML
	}
	if in.IpsLink != "" {
		payload["ipsLink"] = in.IpsLink
	}
	if in.Session != "" {
		payload["session"] = in.Session
	}
	var out PayloadResult
	if err := c.post(ctx, "/payload", payload, &out); err != nil {
		return nil, err
	}
	if len(out.Payload) == 0 || out.TlURL == "" {
		return nil, errors.New("kasada: the API answered without a payload")
	}
	return &out, nil
}

func (c *Client) post(ctx context.Context, route string, payload any, out any) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("kasada: marshal request: %w", err)
	}
	compressed := false
	if len(body) >= compressAbove {
		var buf bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
		if _, err := zw.Write(body); err == nil && zw.Close() == nil {
			body, compressed = buf.Bytes(), true
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint+route, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("kasada: build request: %w", err)
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
		return fmt.Errorf("kasada: request: %w", err)
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("kasada: read response: %w", err)
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
		return fmt.Errorf("kasada: decode response: %w", err)
	}
	return nil
}
