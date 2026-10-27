package datadome

import "time"

// Tags events, sent EventCH then EventLE on one Session.
const (
	EventCH = "ch"
	EventLE = "le"
)

// TagsInput holds the inputs for GenerateTags.
type TagsInput struct {
	// Event is EventCH or EventLE.
	Event string
	// Cookie is the current datadome cookie value.
	Cookie string
	// SiteKey is the site's DataDome key (ddjskey).
	SiteKey string
	// Version is the optional tags.js version (ddv).
	Version string
	// Script is the source of the tags.js the page loaded, optional: the service runs that build, also when
	// the site still serves an older one. Only its SHA-256 is sent. Empty: the current build.
	Script string
	// Referer is the page URL; defaults to Session.Referer.
	Referer string
}

// TagsResponse is returned by GenerateTags; Payload is the encoded /js/ POST body.
type TagsResponse struct {
	Payload     string `json:"payload"`
	Session     string `json:"session"`
	UserAgent   string `json:"userAgent"`
	GeneratedAt int64  `json:"generatedAt"`
}

// InterstitialInput holds the inputs for GenerateInterstitial; HTML is the interstitial page.
type InterstitialInput struct {
	HTML string
	// DeviceLink is the URL HTML was fetched from (required).
	DeviceLink string
	// Referer is the blocked page URL; defaults to Session.Referer.
	Referer string
	// FetchedAt is when HTML was fetched.
	FetchedAt time.Time
	// Script is the source of the page's external challenge script (see ChallengeScriptURL);
	// empty when the page inlines it. Only its SHA-256 is sent.
	Script string
}

// InterstitialResponse is returned by GenerateInterstitial; FormBody (= Payload) is the form to post.
type InterstitialResponse struct {
	Payload   string `json:"payload"`
	Session   string `json:"session"`
	FormBody  string `json:"-"`
	UserAgent string `json:"userAgent"`
}

// SliderInput holds the inputs for GenerateSlider; HTML is the captcha page.
type SliderInput struct {
	HTML string
	// CaptchaURL is the URL HTML was fetched from (required).
	CaptchaURL string
	// ParentURL is the page that showed the captcha; empty derives it from HTML.
	ParentURL string
	// FetchedAt is when HTML was fetched.
	FetchedAt time.Time
	// Script is the page's external captcha script; see InterstitialInput.Script.
	Script string
}

// SliderResponse is returned by GenerateSlider; CheckURL (= Payload) is the /captcha/check URL to GET.
type SliderResponse struct {
	Payload   string `json:"payload"`
	Session   string `json:"session"`
	CheckURL  string `json:"-"`
	UserAgent string `json:"userAgent"`
}
