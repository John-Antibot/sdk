package flow

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// wire is one request Chrome 155 (macOS, headful) sent, from testdata/chrome155_wire.json:
// seatgeek-live entries are NetLog captures (cookies stripped, queries redacted),
// local-raw entries are the raw bytes a local HTTP/1.1 server read.
type wire struct {
	Name, Proto, Method, URL string
	Headers                  Headers
}

func loadWire(t *testing.T) map[string]wire {
	t.Helper()
	data, err := os.ReadFile("testdata/chrome155_wire.json")
	if err != nil {
		t.Fatal(err)
	}
	var list []wire
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	m := map[string]wire{}
	for _, w := range list {
		m[w.Name] = w
	}
	return m
}

// comparable drops pseudo-headers, and maps a captured cookie to CookieSlot;
// a CookieSlot Chrome had nothing for is dropped (the transport skips it too).
func comparable(h Headers, chromeHasCookie bool) Headers {
	out := Headers{}
	for _, kv := range h {
		switch {
		case strings.HasPrefix(kv[0], ":"):
		case strings.EqualFold(kv[0], "cookie") && kv[1] == CookieSlot && !chromeHasCookie:
		case strings.EqualFold(kv[0], "cookie"):
			out = append(out, [2]string{kv[0], CookieSlot})
		default:
			out = append(out, kv)
		}
	}
	return out
}

func hasCookie(h Headers) bool {
	for _, kv := range h {
		if strings.EqualFold(kv[0], "cookie") {
			return true
		}
	}
	return false
}

func TestHTTP1MatchesChrome(t *testing.T) {
	w := loadWire(t)
	b := DefaultBrowser
	site := "https://seatgeek.com/milwaukee-brewers-tickets/10-23-2026-milwaukee-wisconsin-american-family-field/mlb/18528121"
	local := w["local-reload"].URL
	cases := []struct {
		name string
		req  Request
	}{
		{"interstitial-page", Request{Method: "GET", URL: "https://geo.captcha-delivery.com/interstitial/?x", Headers: b.ChallengePage(site)}},
		{"captcha-page", Request{Method: "GET", URL: "https://geo.captcha-delivery.com/captcha/?x", Headers: b.ChallengePage(site)}},
		{"interstitial-post", Request{Method: "POST", URL: "https://geo.captcha-delivery.com/interstitial/",
			Headers: b.Submit("https://geo.captcha-delivery.com/interstitial/?REDACTED", true), Body: strings.Repeat("a", 10109)}},
		{"captcha-check", Request{Method: "GET", URL: "https://geo.captcha-delivery.com/captcha/check?x",
			Headers: b.Submit("https://geo.captcha-delivery.com/captcha/?REDACTED", false)}},
		{"local-reload", Request{Method: "GET", URL: local, Headers: b.Reload()}},
		{"local-navigate-link", Request{Method: "GET", URL: w["local-navigate-link"].URL, Headers: b.Navigate(local)}},
	}
	for _, c := range cases {
		chrome := w[c.name]
		if chrome.Proto != "h1" {
			t.Fatalf("%s: fixture missing", c.name)
		}
		got, want := comparable(c.req.HTTP1(), hasCookie(chrome.Headers)), comparable(chrome.Headers, true)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %q\nwant %q", c.name, got, want)
		}
	}
}

func TestH2MatchesChrome(t *testing.T) {
	w := loadWire(t)
	b := DefaultBrowser
	site := "https://seatgeek.com/milwaukee-brewers-tickets/10-23-2026-milwaukee-wisconsin-american-family-field/mlb/18528121"
	for name, h := range map[string]Headers{
		"interstitial-script": b.ChallengeScript(),
		"captcha-script":      b.ChallengeScript(),
		"site-script":         b.Script(site),
	} {
		chrome := w[name]
		if got, want := comparable(h, hasCookie(chrome.Headers)), comparable(chrome.Headers, true); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %q\nwant %q", name, got, want)
		}
	}
}

func TestHTTP1Rules(t *testing.T) {
	h2 := Headers{
		{"host", "evil"}, {"connection", "close"}, {"content-length", "9"},
		{"sec-ch-ua", "x"}, {"user-agent", "ua"}, {"cookie", CookieSlot}, {"priority", "u=1"},
	}
	before := append(Headers(nil), h2...)
	got := HTTP1(h2, "GET", "https://example.com:8443/a?b", 0)
	want := Headers{{"Host", "example.com:8443"}, {"Connection", "keep-alive"}, {"sec-ch-ua", "x"}, {"User-Agent", "ua"}, {"Cookie", CookieSlot}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	if !reflect.DeepEqual(h2, before) {
		t.Errorf("HTTP1 changed its input: %q", h2)
	}
	if got := HTTP1(nil, "POST", "https://example.com:443/", 0); !reflect.DeepEqual(got, Headers{{"Host", "example.com"}, {"Connection", "keep-alive"}, {"Content-Length", "0"}}) {
		t.Errorf("empty POST: %q", got)
	}
}
