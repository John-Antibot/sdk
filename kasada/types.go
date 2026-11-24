package kasada

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

// PayloadInput is one Kasada fingerprint solve: the ips.js the site
// served, where it was served from, and the browser/IP identity the
// resulting /tl request will use.
type PayloadInput struct {
	// Script is the ips.js body, exactly as fetched.
	Script string
	// HTML is the block page (a 429 page) or /fp body that named ips.js.
	// Send HTML or IpsLink.
	HTML string
	// IpsLink is the ips.js path and query from that page.
	IpsLink string
	// PageURL is the URL of the block/fp page.
	PageURL string
	// UserAgent is the browser fetching and posting as. The result's
	// UserAgent is the collected device actually used.
	UserAgent string
	// IP is the address the /tl request will be posted from.
	IP string
	// AcceptLanguage is sent by that browser, for example en-US,en;q=0.9.
	AcceptLanguage string
	// Session keeps one browser identity across calls. Leave empty for
	// a new identity; send a result's Session to continue it.
	Session string
}

// VersionInput is the p.js a page loaded and the page it loaded on.
type VersionInput struct {
	// Script is the p.js body, exactly as fetched.
	Script string
	// PageURL is the URL of the page that loaded p.js.
	PageURL string
	// UserAgent optionally chooses the collected browser to run as.
	UserAgent string
	// AcceptLanguage is that browser's Accept-Language value.
	AcceptLanguage string
	// Session keeps one browser identity across calls.
	Session string
}

// VersionResult is the x-kpsdk-v value p.js stamps on /fp, and what
// x-kpsdk-cd is made from (see CDHeader).
type VersionResult struct {
	Version string `json:"version"`
	Payload string `json:"payload"`
	// CD is nil when p.js's proof of work could not be read.
	CD        *CDParams `json:"cd,omitempty"`
	Session   string    `json:"session"`
	UserAgent string    `json:"userAgent"`
}

// PayloadResult is the artifact ready for the caller's /tl POST.
type PayloadResult struct {
	// Payload is the decoded binary /tl request body.
	Payload []byte `json:"-"`
	// PayloadBase64 is Payload as returned by the API.
	PayloadBase64 string `json:"payloadBase64"`
	// Headers are the x-kpsdk-* headers for the /tl POST.
	Headers map[string]string `json:"headers"`
	// TlURL is the absolute /tl endpoint.
	TlURL string `json:"tlUrl"`
	// Session continues this browser identity in the next call.
	Session string `json:"session"`
	// UserAgent is the browser the payload was made as.
	UserAgent string `json:"userAgent"`
}

func (r *PayloadResult) UnmarshalJSON(data []byte) error {
	type wire PayloadResult
	var w wire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	decoded, err := base64.StdEncoding.DecodeString(w.PayloadBase64)
	if err != nil {
		return fmt.Errorf("kasada: payloadBase64 is not base64: %w", err)
	}
	*r = PayloadResult(w)
	r.Payload = decoded
	return nil
}

// HTTPHeaders returns Headers ready to merge into the /tl request.
func (r *PayloadResult) HTTPHeaders() http.Header {
	h := http.Header{}
	if r == nil {
		return h
	}
	for name, value := range r.Headers {
		if value != "" {
			h.Set(name, value)
		}
	}
	return h
}
