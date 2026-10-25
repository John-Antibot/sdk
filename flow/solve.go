package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/John-Antibot/sdk/datadome"
)

// SolveOptions tunes a solve flow.
type SolveOptions struct {
	// Browser builds every header set; zero means DefaultBrowser.
	// The session's UserAgent overrides its UserAgent.
	Browser Browser
}

func (o *SolveOptions) browser(s *datadome.Session) Browser {
	b := DefaultBrowser
	if o != nil && (o.Browser != Browser{}) {
		b = o.Browser
	}
	if s != nil && s.UserAgent != "" {
		b.UserAgent = s.UserAgent
	}
	return b
}

// Solve solves whichever challenge the 403 body from pageURL carries.
// cookie is the current datadome cookie, used when the block page omits it.
func Solve(ctx context.Context, t Transport, s *datadome.Session, pageURL, body, cookie string, opts *SolveOptions) (*Result, error) {
	link, captcha, ok := datadome.ChallengeLink(body, pageURL, cookie)
	if !ok {
		return nil, fmt.Errorf("flow: no datadome challenge in the 403 body")
	}
	if u, err := url.Parse(link); err == nil && u.Query().Get("t") == "bv" {
		return nil, fmt.Errorf("flow: block page is a hard block (t=bv); rotate the session")
	}
	if captcha {
		return solveSliderAt(ctx, t, s, pageURL, link, opts)
	}
	return solveInterstitialAt(ctx, t, s, pageURL, link, opts)
}

type geoReply struct {
	View   string `json:"view"`
	Cookie string `json:"cookie"`
	URL    string `json:"url"`
}

// Result is one solved challenge.
type Result struct {
	// View is "redirect" on a solve, "captcha" on escalation to the slider.
	View string
	// Cookie is the reply's datadome cookie, already stored in the jar.
	Cookie *http.Cookie
	// RedirectURL is the reply's target on a redirect.
	RedirectURL string
}

// storeReplyCookie stores the reply's cookie; one without a Domain belongs
// to pageURL's domain, replacing the cookie the block was issued for.
func storeReplyCookie(t Transport, reply geoReply, pageURL string) (*http.Cookie, error) {
	if reply.Cookie == "" {
		return nil, fmt.Errorf("flow: reply carried no cookie")
	}
	cookie, err := http.ParseSetCookie(reply.Cookie)
	if err != nil {
		return nil, fmt.Errorf("flow: parse reply cookie: %w", err)
	}
	if cookie.Domain == "" {
		u, err := url.Parse(pageURL)
		if err != nil || u.Hostname() == "" {
			return nil, fmt.Errorf("flow: reply cookie has no domain and %q has no host", pageURL)
		}
		cookie.Domain = "." + strings.TrimPrefix(u.Hostname(), "www.")
	}
	t.SetCookie(cookie.Domain, cookie.Name, cookie.Value)
	return cookie, nil
}

// SolveInterstitial solves the interstitial for a block page parsed with datadome.ParseDD.
func SolveInterstitial(ctx context.Context, t Transport, s *datadome.Session, pageURL string, dd datadome.DDObject, opts *SolveOptions) (*Result, error) {
	if dd.HardBlocked() {
		return nil, fmt.Errorf("flow: block page is a hard block (t=bv); rotate the session")
	}
	return solveInterstitialAt(ctx, t, s, pageURL, datadome.BuildInterstitialURL(dd, pageURL), opts)
}

func solveInterstitialAt(ctx context.Context, t Transport, s *datadome.Session, pageURL, geoURL string, opts *SolveOptions) (*Result, error) {
	b := opts.browser(s)
	fetchedAt := time.Now()
	page, err := t.Do(ctx, &Request{Method: "GET", URL: geoURL, Headers: b.ChallengePage(pageURL)})
	if err != nil {
		return nil, fmt.Errorf("flow: interstitial page: %w", err)
	}
	if page.Status != 200 {
		return nil, fmt.Errorf("flow: interstitial page returned %d", page.Status)
	}
	script, err := loadChallengeScript(ctx, t, b, page.Body)
	if err != nil {
		return nil, err
	}
	gen, err := s.GenerateInterstitial(ctx, &datadome.InterstitialInput{
		HTML:       page.Body,
		DeviceLink: geoURL,
		Referer:    pageURL,
		FetchedAt:  fetchedAt,
		Script:     script,
	})
	if err != nil {
		return nil, err
	}
	post, err := t.Do(ctx, &Request{
		Method:  "POST",
		URL:     datadome.GeoOrigin + "/interstitial/",
		Headers: b.Submit(geoURL, true),
		Body:    gen.FormBody,
	})
	if err != nil {
		return nil, fmt.Errorf("flow: interstitial submit: %w", err)
	}
	var reply geoReply
	if err := json.Unmarshal([]byte(post.Body), &reply); err != nil {
		return nil, fmt.Errorf("flow: interstitial reply %d: %s", post.Status, truncate(post.Body))
	}
	cookie, err := storeReplyCookie(t, reply, pageURL)
	if err != nil {
		return nil, err
	}
	return &Result{View: reply.View, Cookie: cookie, RedirectURL: reply.URL}, nil
}

// SolveSlider solves the slider captcha for a block page parsed with datadome.ParseDD.
func SolveSlider(ctx context.Context, t Transport, s *datadome.Session, pageURL string, dd datadome.DDObject, opts *SolveOptions) (*Result, error) {
	if dd.HardBlocked() {
		return nil, fmt.Errorf("flow: block page is a hard block (t=bv); rotate the session")
	}
	return solveSliderAt(ctx, t, s, pageURL, datadome.BuildCaptchaURL(dd, pageURL), opts)
}

func solveSliderAt(ctx context.Context, t Transport, s *datadome.Session, pageURL, captchaURL string, opts *SolveOptions) (*Result, error) {
	b := opts.browser(s)
	fetchedAt := time.Now()
	page, err := t.Do(ctx, &Request{Method: "GET", URL: captchaURL, Headers: b.ChallengePage(pageURL)})
	if err != nil {
		return nil, fmt.Errorf("flow: captcha page: %w", err)
	}
	if page.Status != 200 {
		return nil, fmt.Errorf("flow: captcha page returned %d", page.Status)
	}
	script, err := loadChallengeScript(ctx, t, b, page.Body)
	if err != nil {
		return nil, err
	}
	gen, err := s.GenerateSlider(ctx, &datadome.SliderInput{
		HTML:       page.Body,
		CaptchaURL: captchaURL,
		ParentURL:  pageURL,
		FetchedAt:  fetchedAt,
		Script:     script,
	})
	if err != nil {
		return nil, err
	}
	checkURL := gen.CheckURL
	check, err := t.Do(ctx, &Request{Method: "GET", URL: checkURL, Headers: b.Submit(captchaURL, false)})
	if err != nil {
		return nil, fmt.Errorf("flow: captcha check: %w", err)
	}
	var reply geoReply
	if err := json.Unmarshal([]byte(check.Body), &reply); err != nil {
		return nil, fmt.Errorf("flow: captcha check reply %d: %s", check.Status, truncate(check.Body))
	}
	cookie, err := storeReplyCookie(t, reply, pageURL)
	if err != nil {
		return nil, err
	}
	return &Result{View: reply.View, Cookie: cookie, RedirectURL: reply.URL}, nil
}

// SendTags generates and posts one tags payload to jsURL, stores the
// rotated cookie, and returns the POST's HTTP status.
func SendTags(ctx context.Context, t Transport, s *datadome.Session, jsURL string, in *datadome.TagsInput, opts *SolveOptions) (int, error) {
	if in == nil {
		return 0, fmt.Errorf("flow: TagsInput is nil")
	}
	b := opts.browser(s)
	gen, err := s.GenerateTags(ctx, in)
	if err != nil {
		return 0, err
	}
	origin := ""
	if u, err := url.Parse(nonEmptyReferer(in.Referer, s)); err == nil && u.Host != "" {
		origin = u.Scheme + "://" + u.Host
	}
	resp, err := t.Do(ctx, &Request{
		Method:  "POST",
		URL:     jsURL,
		Headers: b.TagsPost(nonEmptyReferer(in.Referer, s), origin),
		Body:    gen.Payload,
	})
	if err != nil {
		return 0, fmt.Errorf("flow: tags post: %w", err)
	}
	var reply geoReply
	if json.Unmarshal([]byte(resp.Body), &reply) == nil && reply.Cookie != "" {
		if cookie, err := http.ParseSetCookie(reply.Cookie); err == nil {
			t.SetCookie(cookie.Domain, cookie.Name, cookie.Value)
		}
	}
	return resp.Status, nil
}

func nonEmptyReferer(referer string, s *datadome.Session) string {
	if referer != "" {
		return referer
	}
	if s != nil {
		return s.Referer
	}
	return ""
}

func truncate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 160 {
		return s[:160]
	}
	return s
}

// loadChallengeScript fetches the page's external challenge script, or "" if inlined.
func loadChallengeScript(ctx context.Context, t Transport, b Browser, html string) (string, error) {
	u := datadome.ChallengeScriptURL(html)
	if u == "" {
		return "", nil
	}
	res, err := t.Do(ctx, &Request{Method: "GET", URL: u, Headers: b.ChallengeScript()})
	if err != nil {
		return "", fmt.Errorf("flow: challenge script: %w", err)
	}
	if res.Status != 200 {
		return "", fmt.Errorf("flow: challenge script returned %d", res.Status)
	}
	return res.Body, nil
}
