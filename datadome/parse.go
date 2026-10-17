package datadome

import (
	"net/url"
	"regexp"
	"strings"
)

// GeoOrigin serves the interstitial and captcha pages.
const GeoOrigin = "https://geo.captcha-delivery.com"

// DDObject is the `var dd = {...}` block on a blocked page.
type DDObject struct {
	RT     string // "i" = interstitial, "c" = captcha
	CID    string // initialCid
	Hash   string // hsh
	T      string // "" solvable, "fe" slider escalation, "bv" hard block
	S      string
	E      string
	B      string
	Cookie string
}

// ParseDD extracts the dd object from a blocked page body.
func ParseDD(html string) DDObject {
	get := func(k string) string {
		m := regexp.MustCompile(`['"]` + regexp.QuoteMeta(k) + `['"]\s*:\s*(?:['"]([^'"]*)['"]|(-?[0-9]+))`).FindStringSubmatch(html)
		if m == nil {
			return ""
		}
		return nonEmpty(m[1], m[2])
	}
	return DDObject{
		RT: get("rt"), CID: get("cid"), Hash: get("hsh"), T: get("t"),
		S: get("s"), E: get("e"), B: get("b"), Cookie: get("cookie"),
	}
}

// HardBlocked reports whether the block is a hard block (t=bv).
func (dd DDObject) HardBlocked() bool { return dd.T == "bv" }

// BuildInterstitialURL builds the interstitial URL for dd blocked on page referer.
func BuildInterstitialURL(dd DDObject, referer string) string {
	q := orderedQuery([][2]string{
		{"initialCid", dd.CID}, {"hash", dd.Hash}, {"cid", dd.Cookie},
		{"referer", referer}, {"s", dd.S},
	})
	if dd.E != "" {
		q += "&e=" + url.QueryEscape(dd.E)
	}
	q += "&b=" + url.QueryEscape(dd.B) + "&dm=cd"
	return GeoOrigin + "/interstitial/?" + q
}

// BuildCaptchaURL builds the captcha URL for dd blocked on page referer.
func BuildCaptchaURL(dd DDObject, referer string) string {
	q := orderedQuery([][2]string{
		{"initialCid", dd.CID}, {"hash", dd.Hash}, {"cid", dd.Cookie},
		{"t", dd.T}, {"referer", referer}, {"s", dd.S},
	})
	if dd.E != "" {
		q += "&e=" + url.QueryEscape(dd.E)
	}
	q += "&dm=cd"
	return GeoOrigin + "/captcha/?" + q
}

// ParseDDM extracts the `var ddm = {...}` values (plus dd-page-size as "ps") from a challenge page.
func ParseDDM(html string) map[string]string {
	out := map[string]string{}
	m := regexp.MustCompile(`var ddm = \{([\s\S]*?)\};`).FindStringSubmatch(html)
	if m == nil {
		return out
	}
	for _, fm := range regexp.MustCompile(`(\w+)\s*:\s*'([^']*)'`).FindAllStringSubmatch(m[1], -1) {
		out[fm[1]] = fm[2]
	}
	for _, fm := range regexp.MustCompile(`(\w+)\s*:\s*(-?\d+)`).FindAllStringSubmatch(m[1], -1) {
		if _, ok := out[fm[1]]; !ok {
			out[fm[1]] = fm[2]
		}
	}
	for _, fm := range regexp.MustCompile(`(\w+)\s*:\s*null`).FindAllStringSubmatch(m[1], -1) {
		if _, ok := out[fm[1]]; !ok {
			out[fm[1]] = "0"
		}
	}
	if pm := regexp.MustCompile(`<meta\s+name="dd-page-size"\s+content="(\d+)"`).FindStringSubmatch(html); pm != nil {
		out["ps"] = pm[1]
	}
	if sm := regexp.MustCompile(`captchaChallengeSeed:\s*'([^']+)'`).FindStringSubmatch(html); sm != nil {
		if out["seed"] == "" {
			out["seed"] = sm[1]
		}
	}
	return out
}

// ParseChallengeScriptURL returns the page's external script URL; ok is false if none.
func ParseChallengeScriptURL(html string) (string, bool) {
	m := regexp.MustCompile(`<script[^>]+src="(https:[^"]*\.js)"`).FindStringSubmatch(html)
	if m == nil {
		return "", false
	}
	return m[1], true
}

var hardblockPageRe = regexp.MustCompile(`responseType\s*=\s*['"]hardblock['"]`)

// IsHardblockPage reports whether a captcha page is the unsolvable hardblock variant.
func IsHardblockPage(html string) bool { return hardblockPageRe.MatchString(html) }

func orderedQuery(pairs [][2]string) string {
	parts := make([]string, 0, len(pairs))
	for _, kv := range pairs {
		parts = append(parts, url.QueryEscape(kv[0])+"="+url.QueryEscape(kv[1]))
	}
	return strings.Join(parts, "&")
}

var challengeScriptRe = regexp.MustCompile(`<script[^>]+src="(https://ct\.captcha-delivery\.com/(?:interstitial|captcha)[^"]*\.js)"`)

// ChallengeScriptURL returns the challenge script URL a page loads, or "" if inlined.
func ChallengeScriptURL(html string) string {
	if m := challengeScriptRe.FindStringSubmatch(html); m != nil {
		return m[1]
	}
	return ""
}

var challengeLinkRe = regexp.MustCompile(`https://geo\.captcha-delivery\.com/(?:captcha|interstitial)/\?[^"'\\\s<]+`)

// ChallengeLink finds the challenge URL in a 403 body; captcha reports a slider link
// and ok is false when there is none.
func ChallengeLink(body, pageURL, cookie string) (link string, captcha, ok bool) {
	body = strings.ReplaceAll(body, `\/`, "/")
	if link = challengeLinkRe.FindString(body); link != "" {
		link = strings.ReplaceAll(link, "&amp;", "&")
		if !strings.Contains(link, "&dm=") {
			mode := "cd"
			if strings.HasPrefix(strings.TrimSpace(body), "{") {
				mode = "jd"
			}
			link += "&dm=" + mode
		}
		return link, strings.Contains(link, "/captcha/"), true
	}
	dd := ParseDD(body)
	if dd.CID == "" || dd.Hash == "" {
		return "", false, false
	}
	if dd.Cookie == "" {
		dd.Cookie = cookie
	}
	switch dd.RT {
	case "c":
		return BuildCaptchaURL(dd, pageURL), true, true
	case "i":
		return BuildInterstitialURL(dd, pageURL), false, true
	}
	return "", false, false
}
