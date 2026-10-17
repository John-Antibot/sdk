// Package flow runs complete DataDome challenge flows over a pluggable Transport.
package flow

import (
	"context"
	"net/http"
	"net/url"

	"github.com/John-Avenue/john-antibot-sdk/datadome"
)

// Transport is the target-site HTTP transport a flow runs over.
type Transport interface {
	// Do sends req with headers in slice order, skipping empty values,
	// filling CookieSlot from its jar and storing Set-Cookie replies.
	Do(ctx context.Context, req *Request) (*Response, error)
	// SetCookie stores a cookie for domain and its subdomains.
	SetCookie(domain, name, value string)
	// Cookie returns the stored value of name for rawURL's host, or "".
	Cookie(rawURL, name string) string
}

// Request is one transport-agnostic HTTP request.
type Request struct {
	Method  string
	URL     string
	Headers Headers
	Body    string
}

// Response is one transport-agnostic HTTP response.
type Response struct {
	Status int
	Header http.Header
	// URL is the final URL after redirects, if tracked.
	URL  string
	Body string
}

// Headers is an ordered header list; a "cookie" entry valued CookieSlot marks the jar cookie's position.
type Headers [][2]string

// CookieSlot marks the cookie position in a header set.
const CookieSlot = "\x00cookie-slot"

// Browser is the identity header sets are built from.
// For a Chrome UserAgent, client hints are derived from it and SecChUA and Platform are ignored.
type Browser struct {
	UserAgent      string
	SecChUA        string
	Platform       string // sec-ch-ua-platform, quoted
	AcceptLanguage string
}

// DefaultBrowser is the latest Chrome on macOS.
var DefaultBrowser = Browser{
	UserAgent:      datadome.LatestUserAgent,
	SecChUA:        datadome.SecCHUA(datadome.LatestUserAgent),
	Platform:       datadome.SecCHUAPlatform(datadome.LatestUserAgent),
	AcceptLanguage: "en-US,en;q=0.9",
}

func (b Browser) chUA() string {
	if v := datadome.SecCHUA(b.UserAgent); v != "" {
		return v
	}
	return b.SecChUA
}

func (b Browser) platform() string {
	if datadome.ChromeMajor(b.UserAgent) != 0 {
		if v := datadome.SecCHUAPlatform(b.UserAgent); v != "" {
			return v
		}
	}
	return b.Platform
}

const (
	documentAccept = "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"
	acceptEncoding = "gzip, deflate, br, zstd"
)

// originReferer reduces pageURL to its origin, as Chrome does cross-origin.
func originReferer(pageURL string) string {
	u, err := url.Parse(pageURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return pageURL
	}
	return u.Scheme + "://" + u.Host + "/"
}

func (b Browser) tail(h Headers, priority string) Headers {
	return append(h,
		[2]string{"accept-encoding", acceptEncoding},
		[2]string{"accept-language", b.AcceptLanguage},
		[2]string{"cookie", CookieSlot},
		[2]string{"priority", priority},
	)
}

// Navigate is a top-level page load: a prerender with an empty referer,
// otherwise a same-origin navigation.
func (b Browser) Navigate(referer string) Headers {
	h := Headers{
		{"sec-ch-ua", b.chUA()},
		{"sec-ch-ua-mobile", "?0"},
		{"sec-ch-ua-platform", b.platform()},
		{"upgrade-insecure-requests", "1"},
		{"user-agent", b.UserAgent},
	}
	if referer == "" {
		h = append(h,
			[2]string{"sec-purpose", "prefetch;prerender"},
			[2]string{"accept", documentAccept},
			[2]string{"sec-fetch-site", "none"},
		)
	} else {
		h = append(h,
			[2]string{"accept", documentAccept},
			[2]string{"sec-fetch-site", "same-origin"},
		)
	}
	h = append(h,
		[2]string{"sec-fetch-mode", "navigate"},
		[2]string{"sec-fetch-user", "?1"},
		[2]string{"sec-fetch-dest", "document"},
	)
	if referer != "" {
		h = append(h, [2]string{"referer", referer})
	}
	return b.tail(h, "u=0, i")
}

// Reload is a same-document reload.
func (b Browser) Reload() Headers {
	h := Headers{
		{"cache-control", "max-age=0"},
		{"sec-ch-ua", b.chUA()},
		{"sec-ch-ua-mobile", "?0"},
		{"sec-ch-ua-platform", b.platform()},
		{"upgrade-insecure-requests", "1"},
		{"user-agent", b.UserAgent},
		{"accept", documentAccept},
		{"sec-fetch-site", "none"},
		{"sec-fetch-mode", "navigate"},
		{"sec-fetch-user", "?1"},
		{"sec-fetch-dest", "document"},
	}
	return b.tail(h, "u=0, i")
}

// ChallengePage is the cross-site iframe load of a challenge page embedded in referer.
func (b Browser) ChallengePage(referer string) Headers {
	referer = originReferer(referer)
	return Headers{
		{"sec-ch-ua", b.chUA()},
		{"sec-ch-ua-mobile", "?0"},
		{"sec-ch-ua-platform", b.platform()},
		{"upgrade-insecure-requests", "1"},
		{"user-agent", b.UserAgent},
		{"accept", documentAccept},
		{"sec-fetch-site", "cross-site"},
		{"sec-fetch-mode", "navigate"},
		{"sec-fetch-dest", "iframe"},
		{"sec-fetch-storage-access", "active"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", b.AcceptLanguage},
		{"cookie", CookieSlot},
	}
}

// ChallengeScript is the challenge frame's load of its script.
func (b Browser) ChallengeScript() Headers {
	return Headers{
		{"sec-ch-ua-platform", b.platform()},
		{"user-agent", b.UserAgent},
		{"sec-ch-ua", b.chUA()},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", "*/*"},
		{"sec-fetch-site", "same-site"},
		{"sec-fetch-mode", "no-cors"},
		{"sec-fetch-dest", "script"},
		{"sec-fetch-storage-access", "active"},
		{"referer", "https://geo.captcha-delivery.com/"},
		{"accept-encoding", acceptEncoding},
		{"accept-language", b.AcceptLanguage},
	}
}

// Submit is the interstitial form POST or slider check GET.
func (b Browser) Submit(referer string, interstitial bool) Headers {
	h := Headers{
		{"sec-ch-ua-platform", b.platform()},
		{"user-agent", b.UserAgent},
		{"sec-ch-ua", b.chUA()},
		{"content-type", "application/x-www-form-urlencoded; charset=UTF-8"},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", "*/*"},
	}
	if interstitial {
		h = append(h, [2]string{"origin", "https://geo.captcha-delivery.com"})
	}
	h = append(h,
		[2]string{"sec-fetch-site", "same-origin"},
		[2]string{"sec-fetch-mode", "cors"},
		[2]string{"sec-fetch-dest", "empty"},
		[2]string{"sec-fetch-storage-access", "active"},
		[2]string{"referer", referer},
		[2]string{"accept-encoding", acceptEncoding},
		[2]string{"accept-language", b.AcceptLanguage},
		[2]string{"cookie", CookieSlot},
		[2]string{"priority", "u=1, i"},
	)
	return h
}

// TagsPost is the /js/ tags POST from page referer on site origin.
func (b Browser) TagsPost(referer, origin string) Headers {
	referer = originReferer(referer)
	return Headers{
		{"sec-ch-ua-platform", b.platform()},
		{"user-agent", b.UserAgent},
		{"sec-ch-ua", b.chUA()},
		{"content-type", "application/x-www-form-urlencoded"},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", "*/*"},
		{"origin", origin},
		{"sec-fetch-site", "same-site"},
		{"sec-fetch-mode", "cors"},
		{"sec-fetch-dest", "empty"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", b.AcceptLanguage},
		{"cookie", CookieSlot},
		{"priority", "u=1, i"},
	}
}

// Script is a cross-site <script src> load from page referer.
func (b Browser) Script(referer string) Headers {
	referer = originReferer(referer)
	return Headers{
		{"sec-ch-ua-platform", b.platform()},
		{"user-agent", b.UserAgent},
		{"sec-ch-ua", b.chUA()},
		{"sec-ch-ua-mobile", "?0"},
		{"accept", "*/*"},
		{"sec-fetch-site", "cross-site"},
		{"sec-fetch-mode", "no-cors"},
		{"sec-fetch-dest", "script"},
		{"sec-fetch-storage-access", "active"},
		{"referer", referer},
		{"accept-encoding", acceptEncoding},
		{"accept-language", b.AcceptLanguage},
		{"cookie", CookieSlot},
		{"priority", "u=1"},
	}
}
