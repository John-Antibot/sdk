package datadome

import (
	"strings"
	"testing"
)

func TestChallengeLinkDisplayMode(t *testing.T) {
	// API wall: JSON 403, the tag shows it with dm=jd.
	link, captcha, ok := ChallengeLink(`{"url":"https:\/\/geo.captcha-delivery.com\/interstitial\/?initialCid=a&cid=b&hash=h&t=it"}`, "https://seatgeek.com/api/x", "")
	if !ok || captcha || !strings.HasSuffix(link, "&dm=jd") {
		t.Fatalf("API wall = %q %v %v; want an interstitial link ending dm=jd", link, captcha, ok)
	}
	// Document 403 with an iframe: dm=cd; a link naming its mode is kept.
	link, captcha, ok = ChallengeLink(`<iframe src="https://geo.captcha-delivery.com/captcha/?cid=b&amp;t=fe"></iframe>`, "", "")
	if !ok || !captcha || link != "https://geo.captcha-delivery.com/captcha/?cid=b&t=fe&dm=cd" {
		t.Fatalf("document = %q %v %v", link, captcha, ok)
	}
	if link, _, _ = ChallengeLink(`{"url":"https://geo.captcha-delivery.com/interstitial/?cid=b&dm=cd"}`, "", ""); strings.Count(link, "dm=") != 1 {
		t.Fatalf("kept mode = %q", link)
	}
	// var dd block page: built, with the jar's cookie when the page has none.
	link, captcha, ok = ChallengeLink(`var dd={'rt':'i','cid':'C','hsh':'H','b':1,'s':2}`, "https://seatgeek.com/", "jar")
	if !ok || captcha || !strings.Contains(link, "cid=jar") || !strings.HasSuffix(link, "&dm=cd") {
		t.Fatalf("dd page = %q %v %v", link, captcha, ok)
	}
	if _, _, ok = ChallengeLink(`<html>no challenge</html>`, "", ""); ok {
		t.Fatal("found a challenge in a page without one")
	}
}
