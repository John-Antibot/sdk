package datadome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Session is one browser identity, created with Client.Session.
// It is not safe for concurrent use.
type Session struct {
	client *Client

	// Session is the service's session token; set it only to resume one.
	Session string
	// SessionKey is an alias for Session.
	SessionKey string
	// Device optionally pins a device profile by name.
	Device string
	// UserAgent is the browser User-Agent; empty means LatestUserAgent.
	UserAgent string
	// Referer is the default page URL for calls.
	Referer string
	// Timezone (IANA) and TimezoneOffset (see JSOffsetMinutes) are optional overrides.
	Timezone       string
	TimezoneOffset int
}

// Session returns a Session from the client defaults, with non-empty override fields winning.
func (c *Client) Session(overrides Session) *Session {
	s := &Session{
		client:         c,
		Session:        firstNonEmpty(overrides.Session, overrides.SessionKey, c.defaults.Session, c.defaults.SessionKey),
		SessionKey:     firstNonEmpty(overrides.Session, overrides.SessionKey, c.defaults.Session, c.defaults.SessionKey),
		Device:         nonEmpty(overrides.Device, c.defaults.Device),
		UserAgent:      firstNonEmpty(overrides.UserAgent, c.defaults.UserAgent, LatestUserAgent),
		Referer:        nonEmpty(overrides.Referer, c.defaults.Referer),
		Timezone:       nonEmpty(overrides.Timezone, c.defaults.Timezone),
		TimezoneOffset: overrides.TimezoneOffset,
	}
	if s.TimezoneOffset == 0 {
		s.TimezoneOffset = c.defaults.TimezoneOffset
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *Session) learn(session, userAgent string) {
	if session != "" {
		s.Session = session
		s.SessionKey = session
	}
	if s.UserAgent == "" && userAgent != "" {
		s.UserAgent = userAgent
	}
}

func (s *Session) identity() map[string]any {
	m := map[string]any{}
	if token := firstNonEmpty(s.Session, s.SessionKey); token != "" {
		m["session"] = token
		m["sessionKey"] = token
	}
	if s.Device != "" {
		m["device"] = s.Device
	}
	if s.UserAgent != "" {
		m["userAgent"] = s.UserAgent
	}
	if s.Timezone != "" {
		m["timezone"] = s.Timezone
	}
	if s.TimezoneOffset != 0 {
		m["timezoneOffset"] = s.TimezoneOffset
	}
	return m
}

// GenerateTags builds one tags payload.
func (s *Session) GenerateTags(ctx context.Context, in *TagsInput) (*TagsResponse, error) {
	if in == nil {
		return nil, errNilInput("Tags")
	}
	req := s.identity()
	req["cid"] = in.Cookie
	req["jsType"] = in.Event
	if in.SiteKey != "" {
		req["ddk"] = in.SiteKey
	}
	if in.Version != "" {
		req["version"] = in.Version
	}
	if in.Script != "" {
		req["scriptHash"] = scriptHash(in.Script)
	}
	referer := nonEmpty(in.Referer, s.Referer)
	if referer == "" {
		return nil, errors.New("datadome: TagsInput.Referer (or Session.Referer) is required: the page the tags run on")
	}
	req["referer"] = referer
	var out TagsResponse
	if err := s.client.post(ctx, "/tags", req, &out); err != nil {
		return nil, err
	}
	s.learn(out.Session, out.UserAgent)
	return &out, nil
}

// GenerateInterstitial returns the interstitial form to POST to GeoOrigin/interstitial/.
func (s *Session) GenerateInterstitial(ctx context.Context, in *InterstitialInput) (*InterstitialResponse, error) {
	if in == nil {
		return nil, errNilInput("Interstitial")
	}
	if in.DeviceLink == "" {
		return nil, errors.New("datadome: InterstitialInput.DeviceLink is required: the interstitial URL you loaded")
	}
	if err := requireScript(in.HTML, in.Script); err != nil {
		return nil, err
	}
	referer := nonEmpty(in.Referer, s.Referer)

	req := s.identity()
	req["html"] = in.HTML
	req["deviceUrl"] = in.DeviceLink
	if referer != "" {
		req["referer"] = referer
	}
	if !in.FetchedAt.IsZero() {
		req["challengeFetchedAtMs"] = in.FetchedAt.UnixMilli()
	}
	if in.Script != "" {
		req["scriptHash"] = scriptHash(in.Script)
	}
	var wire struct {
		Payload   string `json:"payload"`
		Session   string `json:"session"`
		UserAgent string `json:"userAgent"`
	}
	if err := s.client.post(ctx, "/interstitial", req, &wire); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(wire.Payload, "cid=") {
		return nil, errors.New("datadome: service returned no ready-to-post form; it predates the form-body wire")
	}
	out := InterstitialResponse{Session: wire.Session, UserAgent: wire.UserAgent, Payload: wire.Payload}
	out.FormBody = out.Payload
	s.learn(out.Session, out.UserAgent)
	return &out, nil
}

// GenerateSlider returns the /captcha/check URL to GET.
func (s *Session) GenerateSlider(ctx context.Context, in *SliderInput) (*SliderResponse, error) {
	if in == nil {
		return nil, errNilInput("Slider")
	}
	if in.CaptchaURL == "" {
		return nil, errors.New("datadome: SliderInput.CaptchaURL is required: the captcha URL you loaded")
	}
	if err := requireScript(in.HTML, in.Script); err != nil {
		return nil, err
	}
	req := s.identity()
	req["html"] = in.HTML
	req["captchaUrl"] = in.CaptchaURL
	if in.ParentURL != "" {
		req["parentUrl"] = in.ParentURL
	}
	if !in.FetchedAt.IsZero() {
		req["challengeFetchedAtMs"] = in.FetchedAt.UnixMilli()
	}
	if in.Script != "" {
		req["scriptHash"] = scriptHash(in.Script)
	}
	var wire struct {
		Payload   string `json:"payload"`
		Session   string `json:"session"`
		UserAgent string `json:"userAgent"`
	}
	if err := s.client.post(ctx, "/slider", req, &wire); err != nil {
		return nil, err
	}
	u := normalizeCheckURL(wire.Payload, "")
	if u == "" {
		return nil, errors.New("datadome: service returned no check URL; it predates the check-URL wire")
	}
	out := SliderResponse{Session: wire.Session, UserAgent: wire.UserAgent, Payload: u, CheckURL: u}
	s.learn(out.Session, out.UserAgent)
	return &out, nil
}

// scriptHash is what the service takes of a page script: its SHA-256, as hex.
func scriptHash(script string) string {
	h := sha256.Sum256([]byte(script))
	return hex.EncodeToString(h[:])
}

// requireScript errors when the page loads an external script but none was passed.
func requireScript(html, script string) error {
	if u := ChallengeScriptURL(html); u != "" && script == "" {
		return fmt.Errorf("datadome: Script is required: load %s right after the page and pass its source", u)
	}
	return nil
}
