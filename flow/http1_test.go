package flow

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/John-Antibot/sdk/datadome"
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

// chrome is the browser of a Chrome major on macOS, as DefaultBrowser is for the latest.
func chrome(major int) Browser {
	ua := datadome.ChromeUserAgent(major)
	return Browser{UserAgent: ua, SecChUA: datadome.SecCHUA(ua), Platform: datadome.SecCHUAPlatform(ua), AcceptLanguage: "en-US,en;q=0.9"}
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
	b := chrome(155) // the wire captures are Chrome 155's
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
	b := chrome(155) // the wire captures are Chrome 155's
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

// Chrome 156 sends the headers of 155 with its own user agent and client hints: the recorded 155 and 156 presets differ in no other
// header, and in no TLS or HTTP/2 setting (the 156 preset is based on 155). Its sec-ch-ua is the string Chrome 156 sends, the grease
// brand included.
func TestChrome156IsChrome155WithItsOwnIdentity(t *testing.T) {
	b155, b156 := chrome(155), chrome(156)
	if b156.SecChUA != `"Not:A-Brand";v="8", "Chromium";v="156", "Google Chrome";v="156"` {
		t.Errorf("sec-ch-ua %s", b156.SecChUA)
	}
	if DefaultBrowser.UserAgent != b156.UserAgent {
		t.Errorf("DefaultBrowser is %s, not Chrome 156", DefaultBrowser.UserAgent)
	}
	site := "https://seatgeek.com/some/page"
	swap := func(h Headers) Headers {
		out := Headers{}
		for _, kv := range h {
			switch kv[0] {
			case "user-agent":
				kv[1] = b156.UserAgent
			case "sec-ch-ua":
				kv[1] = b156.SecChUA
			}
			out = append(out, kv)
		}
		return out
	}
	for name, pair := range map[string][2]Headers{
		"navigate":         {b155.Navigate(""), b156.Navigate("")},
		"navigate-referer": {b155.Navigate(site), b156.Navigate(site)},
		"reload":           {b155.Reload(), b156.Reload()},
		"challenge-script": {b155.ChallengeScript(), b156.ChallengeScript()},
		"site-script":      {b155.Script(site), b156.Script(site)},
	} {
		if want := swap(append(Headers(nil), pair[0]...)); !reflect.DeepEqual(pair[1], want) {
			t.Errorf("%s:\n got %q\nwant %q", name, pair[1], want)
		}
	}
}
