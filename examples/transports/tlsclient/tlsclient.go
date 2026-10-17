// Package tlsclient adapts github.com/bogdanfinn/tls-client to flow.Transport.
package tlsclient

import (
	"context"
	"io"
	"net/http"
	"strings"

	fhttp "github.com/bogdanfinn/fhttp"
	bogdan "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"

	"github.com/John-Avenue/john-antibot-sdk/examples/internal/jar"
	"github.com/John-Avenue/john-antibot-sdk/flow"
)

// Transport implements flow.Transport over one tls-client instance.
type Transport struct {
	client bogdan.HttpClient
	jar    *jar.Jar
}

// New returns a Transport on the Chrome 133 profile; proxy is "" or a proxy URL.
func New(proxy string) (*Transport, error) {
	options := []bogdan.HttpClientOption{
		bogdan.WithTimeoutSeconds(45),
		bogdan.WithClientProfile(profiles.Chrome_133),
		bogdan.WithNotFollowRedirects(),
		bogdan.WithDisableHttp3(),
	}
	if proxy != "" {
		options = append(options, bogdan.WithProxyUrl(proxy))
	}
	client, err := bogdan.NewHttpClient(bogdan.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}
	return &Transport{client: client, jar: jar.New()}, nil
}

// Close releases idle connections.
func (t *Transport) Close() { t.client.CloseIdleConnections() }

// Do sends req in header order, filling CookieSlot from the jar.
func (t *Transport) Do(ctx context.Context, req *flow.Request) (*flow.Response, error) {
	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}
	freq, err := fhttp.NewRequestWithContext(ctx, req.Method, req.URL, body)
	if err != nil {
		return nil, err
	}
	order := []string{}
	for _, h := range req.Headers {
		name, value := h[0], h[1]
		if name == "cookie" && value == flow.CookieSlot {
			value = t.jar.Header(req.URL)
			if value == "" {
				continue
			}
		}
		if value == "" {
			continue
		}
		freq.Header.Set(name, value)
		order = append(order, name)
	}
	freq.Header[fhttp.HeaderOrderKey] = order
	res, err := t.client.Do(freq)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	for name, values := range res.Header {
		for _, v := range values {
			header.Add(name, v)
		}
	}
	t.jar.StoreResponseCookies(req.URL, header)
	finalURL := req.URL
	if res.Request != nil && res.Request.URL != nil {
		finalURL = res.Request.URL.String()
	}
	return &flow.Response{Status: res.StatusCode, Header: header, URL: finalURL, Body: string(data)}, nil
}

// SetCookie stores a cookie for domain and subdomains.
func (t *Transport) SetCookie(domain, name, value string) { t.jar.Set(domain, name, value) }

// Cookie returns one stored cookie value for rawURL.
func (t *Transport) Cookie(rawURL, name string) string { return t.jar.Get(rawURL, name) }

var _ flow.Transport = (*Transport)(nil)
