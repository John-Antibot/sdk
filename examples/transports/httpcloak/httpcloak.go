// Package httpcloak adapts github.com/sardanioss/httpcloak to flow.Transport.
package httpcloak

import (
	"context"
	"net/http"
	"strings"
	"time"

	cloak "github.com/sardanioss/httpcloak"

	"github.com/John-Avenue/john-antibot-sdk/examples/internal/jar"
	"github.com/John-Avenue/john-antibot-sdk/flow"
)

// Transport implements flow.Transport over one httpcloak session.
type Transport struct {
	session *cloak.Session
	jar     *jar.Jar
}

// New returns a Transport; proxy is "" or a proxy URL.
func New(proxy string) *Transport {
	options := []cloak.SessionOption{
		cloak.WithoutRedirects(),
		cloak.WithoutRetry(),
		cloak.WithoutCookieJar(),
	}
	if proxy != "" {
		options = append(options, cloak.WithSessionProxy(proxy))
	}
	return &Transport{session: cloak.NewSession("chrome-latest", options...), jar: jar.New()}
}

// Close releases the session.
func (t *Transport) Close() { t.session.Close() }

// Do sends req in header order, filling CookieSlot from the jar.
func (t *Transport) Do(ctx context.Context, req *flow.Request) (*flow.Response, error) {
	headers := map[string][]string{}
	order := []string{"content-length"}
	for _, h := range req.Headers {
		name, value := h[0], h[1]
		if name == "cookie" && value == flow.CookieSlot {
			value = t.jar.Header(req.URL)
			order = append(order, "cookie")
			if value == "" {
				continue
			}
		}
		if value == "" {
			continue
		}
		headers[name] = []string{value}
		order = append(order, name)
	}
	creq := &cloak.Request{Method: req.Method, URL: req.URL, Headers: headers, HeaderOrder: order, Timeout: 45 * time.Second}
	if req.Body != "" {
		creq.Body = strings.NewReader(req.Body)
	}
	res, err := t.session.Do(ctx, creq)
	if err != nil {
		return nil, err
	}
	body, err := res.Text()
	if err != nil {
		return nil, err
	}
	header := http.Header{}
	for name, values := range res.Headers {
		for _, v := range values {
			header.Add(name, v)
		}
	}
	t.jar.StoreResponseCookies(req.URL, header)
	return &flow.Response{Status: res.StatusCode, Header: header, URL: res.FinalURL, Body: body}, nil
}

// SetCookie stores a cookie for domain and subdomains.
func (t *Transport) SetCookie(domain, name, value string) { t.jar.Set(domain, name, value) }

// Cookie returns one stored cookie value for rawURL.
func (t *Transport) Cookie(rawURL, name string) string { return t.jar.Get(rawURL, name) }

var _ flow.Transport = (*Transport)(nil)
