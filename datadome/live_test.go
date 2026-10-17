package datadome

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// Live round-trip against a running generator. Opt in with
// DD_SDK_LIVE=1 (endpoint from DD_GEN, default local).
func TestLiveGenerate(t *testing.T) {
	if os.Getenv("DD_SDK_LIVE") != "1" {
		t.Skip("set DD_SDK_LIVE=1 to run against the live generator")
	}
	client, err := New(Config{Endpoint: os.Getenv("DD_GEN"), APIKey: os.Getenv("DD_API_KEY")})
	if err != nil {
		t.Fatal(err)
	}
	session := client.Session(Session{
		SessionKey: "sdk-live-test", Referer: "https://seatgeek.com/",
	})
	ctx := context.Background()

	tags, err := session.GenerateTags(ctx, &TagsInput{Event: EventCH, Cookie: "DUMMYCOOKIE", SiteKey: "KEY", Referer: "https://seatgeek.com/"})
	if err != nil {
		t.Fatalf("tags ch: %v", err)
	}
	if tags.Payload == "" {
		t.Fatalf("tags ch empty: %+v", tags)
	}

	interHTML, err := os.ReadFile("../../../services/datadome/samples/live-202610141/interstitial.html")
	if err != nil {
		t.Skipf("interstitial sample unavailable: %v", err)
	}
	inter, err := session.GenerateInterstitial(ctx, &InterstitialInput{
		HTML: string(interHTML), Referer: "https://seatgeek.com/",
		FetchedAt: time.Now().Add(-900 * time.Millisecond),
	})
	if err != nil {
		t.Fatalf("interstitial: %v", err)
	}
	if len(inter.Payload) < 4000 || !strings.Contains(inter.Payload, "&plv3=") {
		t.Fatalf("interstitial payload too small or missing embedded plv3: %d", len(inter.Payload))
	}
	if !strings.HasPrefix(inter.FormBody, "cid=") || !strings.Contains(inter.FormBody, "&payload=") {
		t.Fatalf("FormBody malformed: %.80s", inter.FormBody)
	}

	sliderHTML, err := os.ReadFile("../../../services/datadome/samples/slider/live-2026-08-26/page.html")
	if err != nil {
		t.Skipf("slider sample unavailable: %v", err)
	}
	slider, err := session.GenerateSlider(ctx, &SliderInput{
		HTML: string(sliderHTML), CaptchaURL: "https://geo.captcha-delivery.com/captcha/",
		FetchedAt: time.Now(),
	})
	if err != nil {
		t.Fatalf("slider: %v", err)
	}
	if len(slider.Payload) < 3000 || !strings.Contains(slider.CheckURL, "/captcha/check?cid=") {
		t.Fatalf("slider response malformed: payload=%d checkURL=%.80s", len(slider.Payload), slider.CheckURL)
	}
	if session.UserAgent == "" {
		t.Fatal("session did not learn a user agent")
	}
}
