package datadome

import (
	"net/url"
	"regexp"
	"strings"
)

// FormBodyFor returns the form body with its referer and url fields set to pageURL.
// An empty pageURL returns it unchanged.
func (r *InterstitialResponse) FormBodyFor(pageURL string) string {
	body := nonEmpty(r.FormBody, r.Payload)
	if pageURL == "" {
		return body
	}
	return replaceFormFields(body, map[string]string{"referer": pageURL, "url": pageURL})
}

func replaceFormFields(form string, values map[string]string) string {
	if form == "" {
		return form
	}
	parts := strings.Split(form, "&")
	for i, part := range parts {
		key, _, _ := strings.Cut(part, "=")
		if value, ok := values[key]; ok {
			parts[i] = url.QueryEscape(key) + "=" + strings.ReplaceAll(url.QueryEscape(value), "%7E", "~")
		}
	}
	return strings.Join(parts, "&")
}

func replaceQueryField(rawURL, key, value string) string {
	if rawURL == "" || value == "" {
		return rawURL
	}
	base, query, found := strings.Cut(rawURL, "?")
	if !found {
		return rawURL + "?" + url.QueryEscape(key) + "=" + strings.ReplaceAll(url.QueryEscape(value), "%7E", "~")
	}
	return base + "?" + replaceFormFields(query, map[string]string{key: value})
}

func normalizeCheckURL(payload, parentURL string) string {
	if strings.HasPrefix(payload, "http://") || strings.HasPrefix(payload, "https://") {
		return replaceQueryField(payload, "parent_url", parentURL)
	}
	if strings.HasPrefix(payload, "?") {
		return replaceQueryField(GeoOrigin+"/captcha/check"+payload, "parent_url", parentURL)
	}
	return ""
}

// CaptchaPage holds the check parameters a captcha page embeds.
type CaptchaPage struct {
	CID     string
	Hash    string
	UA      string
	S       string
	UserEnv string
	ICID    string
	CCID    string
	// Challenge is the ddCaptchaChallenge echo value.
	Challenge string
	// Env is the ddCaptchaEnv echo value.
	Env string
	// AudioChallenge is the ddCaptchaAudioChallenge echo.
	AudioChallenge string
	Dm             string
	Referer        string
	IR             string
	Seed           string
	NoPuzzle       bool
	// PuzzlePath is the puzzle image path on puzzle-class pages.
	PuzzlePath string
	// Hardblock marks the unsolvable hardblock variant.
	Hardblock bool
}

// ParseCaptchaPage extracts the check parameters from a captcha page body.
func ParseCaptchaPage(html string) CaptchaPage {
	grab := func(re string) string {
		m := regexp.MustCompile(re).FindStringSubmatch(html)
		if m == nil {
			return ""
		}
		return m[1]
	}
	p := CaptchaPage{
		CID:            grab(`var ddm = \{[\s\S]*?cid:\s*'([^']+)'`),
		Hash:           grab(`var ddm = \{[\s\S]*?hash:\s*'([^']+)'`),
		UA:             grab(`var ddm = \{[\s\S]*?ua:\s*'([^']+)'`),
		S:              grab(`var ddm = \{[\s\S]*?\bs:\s*'([^']*)'`),
		UserEnv:        grab(`var ddm = \{[\s\S]*?userEnv:\s*'([^']+)'`),
		ICID:           grab(`'&icid=' \+ encodeURIComponent\( '([^']+)' \)`),
		CCID:           grab(`'&ccid=' \+ encodeURIComponent\( '([^']*)' \)`),
		Challenge:      grab(`'&ddCaptchaChallenge=' \+ encodeURIComponent\( '([^']+)' \)`),
		Env:            grab(`'&ddCaptchaEnv=' \+ encodeURIComponent\( '([^']+)' \)`),
		AudioChallenge: grab(`'&ddCaptchaAudioChallenge=' \+ encodeURIComponent\( '([^']*)' \)`),
		Dm:             grab(`'&dm=' \+ encodeURIComponent\( '([^']*)' \)`),
		Referer:        grab(`ddm\.referer = htmlDecode\( '([^']+)' \)`),
		IR:             grab(`'&ir=' \+ encodeURIComponent\( '([^']*)' \)`),
		Seed:           grab(`captchaChallengeSeed: '([^']+)'`),
		NoPuzzle:       strings.Contains(html, "noPuzzle: true") || strings.Contains(html, `"noPuzzle":true`),
		PuzzlePath:     grab(`captchaChallengePath["']?\s*[:=]\s*['"]([^'"]+)['"]`),
		Hardblock:      IsHardblockPage(html),
	}
	return p
}

// CheckURL returns resp's /captcha/check URL, with parent_url set to parentURL if non-empty.
func (p CaptchaPage) CheckURL(resp *SliderResponse, parentURL string) string {
	if resp == nil {
		return ""
	}
	return normalizeCheckURL(nonEmpty(resp.CheckURL, resp.Payload), parentURL)
}
