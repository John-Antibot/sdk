// Package httpcloak adapts github.com/sardanioss/httpcloak to flow.Transport.
package httpcloak

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	cloak "github.com/sardanioss/httpcloak"

	"github.com/John-Antibot/sdk/examples/internal/jar"
	"github.com/John-Antibot/sdk/flow"
)

// Transport implements flow.Transport over one httpcloak session.
type Transport struct {
	session *cloak.Session
	jar     *jar.Jar
	// http1 holds the hosts that answer over HTTP/1.1: geo.captcha-delivery.com
	// from the start, and every host a response came back on h1 from.
	// Requests to them send req.HTTP1(). Store a host to add one.
	http1 sync.Map
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
	t := &Transport{session: cloak.NewSession("chrome-latest", options...), jar: jar.New()}
	t.http1.Store("geo.captcha-delivery.com", true)
	return t
}

// Close releases the session.
func (t *Transport) Close() { t.session.Close() }

// Do sends req in header order, filling CookieSlot from the jar.
// On HTTP/1.1 httpcloak writes Host, Connection and Content-Length itself,
// in Chrome's places and casing; req.HTTP1() supplies the rest without Priority.
func (t *Transport) Do(ctx context.Context, req *flow.Request) (*flow.Response, error) {
	host := ""
	if u, err := url.Parse(req.URL); err == nil {
		host = u.Hostname()
	}
	list := req.Headers
	if _, h1 := t.http1.Load(host); h1 {
		list = req.HTTP1()
	}
	headers := map[string][]string{}
	order := []string{"content-length"}
	for _, h := range list {
		name, value := h[0], h[1]
		switch strings.ToLower(name) {
		case "host", "connection", "content-length":
			continue
		}
		if strings.EqualFold(name, "cookie") && value == flow.CookieSlot {
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
	if res.Protocol == "h1" {
		t.http1.Store(host, true)
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
