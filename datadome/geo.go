package datadome

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DataDomeCookie is the name of the DataDome session cookie.
const DataDomeCookie = "datadome"

// Verdict values of the t parameter.
const (
	// VerdictSolvable is t=fe, a solvable slider handoff.
	VerdictSolvable = "fe"
	// VerdictHardBlock is t=bv, a hard block.
	VerdictHardBlock = "bv"
)

// GeoAnswer is the parsed reply to an interstitial POST or slider check GET.
type GeoAnswer struct {
	// IsJSON reports whether the body parsed as JSON.
	IsJSON bool
	// View is "redirect" when cleared, "captcha" on a slider handoff.
	View string
	// URL is the redirect target or slider handoff URL.
	URL string
	// T is the t parameter of URL.
	T string
	// IR is the ir parameter of URL.
	IR string
	// RawCookie is the full Set-Cookie string.
	RawCookie string
	// Cookie is the datadome value inside RawCookie.
	Cookie string
	// CookieMaxAge is the lifetime of Cookie.
	CookieMaxAge time.Duration
	// HardBlock is true when T is VerdictHardBlock.
	HardBlock bool
}

// Cleared reports whether the session was cleared with a usable cookie.
func (a GeoAnswer) Cleared() bool { return a.View == "redirect" && a.Cookie != "" }

// ValidateGeo parses a geo.captcha-delivery.com reply body.
func ValidateGeo(body []byte) GeoAnswer {
	var a GeoAnswer
	var geo struct {
		Cookie string `json:"cookie"`
		View   string `json:"view"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(body, &geo); err != nil {
		return a
	}
	a.IsJSON = true
	a.View = strings.ToLower(strings.TrimSpace(geo.View))
	a.URL = geo.URL
	a.RawCookie = geo.Cookie
	a.Cookie, a.CookieMaxAge = readSetCookie(geo.Cookie)
	if geo.URL != "" {
		if u, err := url.Parse(geo.URL); err == nil {
			a.T = strings.ToLower(u.Query().Get("t"))
			a.IR = u.Query().Get("ir")
			a.HardBlock = a.T == VerdictHardBlock
		}
	}
	return a
}

func readSetCookie(line string) (string, time.Duration) {
	c, err := http.ParseSetCookie(line)
	if err != nil || c.Name != DataDomeCookie {
		return "", 0
	}
	return c.Value, time.Duration(c.MaxAge) * time.Second
}
