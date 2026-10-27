package datadome

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const interstitialHTML = `<html><head><meta name="dd-page-size" content="10510">
<script>var ddm = { cid: 'CID~x', hash: 'HASH', userEnv: 'UE', seed: 'SEED', env: 'ENV', s: 43624, b: 1265040, e: 'E1' };</script>
<script src="https://ct.captcha-delivery.com/interstitial.1.0.js"></script></head></html>`

func testServer(t *testing.T, route string, reply string, capture *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != route {
			t.Errorf("route = %s, want %s", r.URL.Path, route)
		}
		body, _ := io.ReadAll(r.Body)
		if capture != nil {
			_ = json.Unmarshal(body, capture)
		}
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, reply)
	}))
}

func TestGenerateInterstitialWireAndForm(t *testing.T) {
	var got map[string]any
	formBody := "cid=CID~x&hash=HASH&referer=https%3A%2F%2Fseatgeek.com%2F&url=https%3A%2F%2Fseatgeek.com%2F&s=43624&e=E1&env=ENV&userEnv=UE&seed=SEED&b=1265040&dm=cd&ddMessageFormat=&payload=PLV2BLOB&plv3=PLV3TOK&ps=10510"
	srv := testServer(t, "/interstitial", `{"payload":"`+formBody+`","session":"sess-1","userAgent":"UA-FROM-PROFILE"}`, &got)
	defer srv.Close()

	client, err := New(Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	session := client.Session(Session{SessionKey: "sess-1", Timezone: "America/New_York", TimezoneOffset: 240})
	fetched := time.UnixMilli(1792000000000)
	out, err := session.GenerateInterstitial(context.Background(), &InterstitialInput{
		HTML: interstitialHTML, DeviceLink: "https://geo.captcha-delivery.com/interstitial/?x=1",
		Referer: "https://seatgeek.com/", FetchedAt: fetched, Script: "/*script*/",
	})
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]any{
		"html": interstitialHTML, "deviceUrl": "https://geo.captcha-delivery.com/interstitial/?x=1", "scriptHash": scriptHash("/*script*/"),
		"referer": "https://seatgeek.com/", "session": "sess-1", "sessionKey": "sess-1", "timezone": "America/New_York",
	} {
		if got[k] != want {
			t.Errorf("wire %s = %v, want %v", k, got[k], want)
		}
	}
	// The service reads what the page carries from html; the SDK sends only the documented fields.
	for _, k := range []string{"cid", "hash", "userEnv", "seed", "env"} {
		if _, sent := got[k]; sent {
			t.Errorf("wire carries %s; the service reads it from html", k)
		}
	}
	if got["challengeFetchedAtMs"] != float64(1792000000000) {
		t.Errorf("challengeFetchedAtMs = %v", got["challengeFetchedAtMs"])
	}
	// The session speaks the latest Chrome by default and keeps it:
	// the service pins a device of that exact version.
	if got["userAgent"] != LatestUserAgent {
		t.Errorf("wire userAgent = %v, want LatestUserAgent", got["userAgent"])
	}
	if session.Session != "sess-1" || session.UserAgent != LatestUserAgent {
		t.Errorf("session token/user agent: %q %q", session.Session, session.UserAgent)
	}
	if out.Session != "sess-1" {
		t.Errorf("response session = %q", out.Session)
	}
	if out.Payload != formBody {
		t.Errorf("Payload = %q, want complete form body", out.Payload)
	}
	form := out.FormBody
	if rebuilt := out.FormBodyFor("https://seatgeek.com/"); rebuilt != form {
		t.Errorf("FormBodyFor mismatch:\n%s\n%s", rebuilt, form)
	}
	for _, want := range []string{"cid=CID~x", "payload=PLV2BLOB", "plv3=PLV3TOK", "ps=10510", "referer=https%3A%2F%2Fseatgeek.com%2F"} {
		if !strings.Contains(form, want) {
			t.Errorf("form missing %q: %s", want, form)
		}
	}
	if strings.Contains(form, "%7E") {
		t.Errorf("form escaped ~: %s", form)
	}
	if !strings.HasPrefix(form, "cid=") {
		t.Errorf("form field order lost: %s", form)
	}
}

func TestGenerateTagsWire(t *testing.T) {
	var got map[string]any
	srv := testServer(t, "/tags", `{"payload":"FORM","session":"sess-new","userAgent":"UA","generatedAt":5}`, &got)
	defer srv.Close()
	client, _ := New(Config{Endpoint: srv.URL})
	session := client.Session(Session{Referer: "https://seatgeek.com/"})
	out, err := session.GenerateTags(context.Background(), &TagsInput{Event: EventCH, Cookie: "cookieval", SiteKey: "KEY"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Payload != "FORM" || got["jsType"] != "ch" || got["cid"] != "cookieval" || got["ddk"] != "KEY" {
		t.Errorf("tags wire wrong: out=%+v got=%v", out, got)
	}
	if _, ok := got["session"]; ok {
		t.Errorf("first call should carry no session, got %v", got["session"])
	}
	if out.Session != "sess-new" || session.Session != "sess-new" {
		t.Errorf("session token not learned: out=%q session=%q", out.Session, session.Session)
	}
	if _, ok := got["scriptHash"]; ok {
		t.Errorf("no Script, no scriptHash: %v", got["scriptHash"])
	}
	if _, err := session.GenerateTags(context.Background(), &TagsInput{Event: EventLE, Cookie: "c", SiteKey: "KEY", Script: "/*tags*/"}); err != nil {
		t.Fatal(err)
	}
	if got["scriptHash"] != scriptHash("/*tags*/") || got["script"] != nil {
		t.Errorf("tags wire scriptHash = %v script = %v", got["scriptHash"], got["script"])
	}
}

func TestAPIErrorClassification(t *testing.T) {
	for _, c := range []struct {
		name, body string
		status     int
		hardblock  bool
	}{
		{"422 hardblock", `{"error":"captcha page is hardblock (no solvable slider)","session":"s","hardblock":true}`, 422, true},
		{"an earlier gateway's 500", `{"error":"captcha page is hardblock (no solvable slider)"}`, 500, true},
		{"a failure", `{"error":"generation failed"}`, 500, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c.status)
				_, _ = io.WriteString(w, c.body)
			}))
			defer srv.Close()
			client, _ := New(Config{Endpoint: srv.URL})
			session := client.Session(Session{})
			_, err := session.GenerateSlider(context.Background(), &SliderInput{HTML: "<html></html>", CaptchaURL: "https://geo.captcha-delivery.com/captcha/?x=1"})
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != c.status {
				t.Fatalf("err = %v, want an APIError %d", err, c.status)
			}
			if IsHardblock(err) != c.hardblock {
				t.Errorf("IsHardblock(%v) = %v, want %v", err, !c.hardblock, c.hardblock)
			}
			if IsPuzzleUnsupported(err) {
				t.Errorf("IsPuzzleUnsupported(%v) = true", err)
			}
		})
	}
}

func TestParsers(t *testing.T) {
	dd := ParseDD(`var dd = {'rt':'i','cid':'C1','hsh':'H1','t':'','s':123,'e':'E','b':456,'cookie':'CK'}`)
	if dd.RT != "i" || dd.CID != "C1" || dd.Hash != "H1" || dd.S != "123" || dd.B != "456" || dd.Cookie != "CK" {
		t.Errorf("ParseDD = %+v", dd)
	}
	u := BuildInterstitialURL(dd, "https://seatgeek.com/")
	if !strings.HasPrefix(u, GeoOrigin+"/interstitial/?initialCid=C1&hash=H1&cid=CK&referer=") || !strings.HasSuffix(u, "&b=456&dm=cd") {
		t.Errorf("BuildInterstitialURL = %s", u)
	}
	if script, ok := ParseChallengeScriptURL(interstitialHTML); !ok || script != "https://ct.captcha-delivery.com/interstitial.1.0.js" {
		t.Errorf("ParseChallengeScriptURL = %q %v", script, ok)
	}
	if !IsHardblockPage(`var responseType = 'hardblock';`) {
		t.Error("IsHardblockPage missed hardblock marker")
	}
}

func TestCaptchaPageCheckURL(t *testing.T) {
	html := `<script>var ddm = { cid: 'C', hash: 'H', ua: 'UA', s: 'S1', userEnv: 'UE', noPuzzle: true };
ddm.referer = htmlDecode( 'https://seatgeek.com/' );
getRequest += '&icid=' + encodeURIComponent( 'IC' );
getRequest += '&ccid=' + encodeURIComponent( '' );
getRequest += '&dm=' + encodeURIComponent( 'dc_ir' );
getRequest += '&ddCaptchaChallenge=' + encodeURIComponent( 'ECH' );
getRequest += '&ddCaptchaEnv=' + encodeURIComponent( 'ENVX' );
getRequest += '&ddCaptchaAudioChallenge=' + encodeURIComponent( 'AUD' );
getRequest += '&ir=' + encodeURIComponent( '503003' );</script>`
	p := ParseCaptchaPage(html)
	if p.CID != "C" || p.Challenge != "ECH" || p.Dm != "dc_ir" || !p.NoPuzzle {
		t.Fatalf("ParseCaptchaPage = %+v", p)
	}
	u := p.CheckURL(&SliderResponse{Payload: GeoOrigin + "/captcha/check?cid=C&icid=IC&ccid=&userEnv=UE&dm=dc_ir&ddCaptchaChallenge=ECH&ddCaptchaEncodedPayload=PAY&plv3=TOK&ir=503003"}, "")
	for _, want := range []string{"/captcha/check?cid=C&icid=IC&ccid=&userEnv=UE&dm=dc_ir&ddCaptchaChallenge=ECH&ddCaptchaEncodedPayload=PAY&plv3=TOK", "&ir=503003"} {
		if !strings.Contains(u, want) {
			t.Errorf("CheckURL missing %q: %s", want, u)
		}
	}
}

func TestJSOffsetMinutes(t *testing.T) {
	at := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	got, err := JSOffsetMinutes("America/New_York", at)
	if err != nil || got != 240 {
		t.Errorf("JSOffsetMinutes = %d, %v; want 240", got, err)
	}
}

func TestInterstitialSendsClientScript(t *testing.T) {
	if got := ChallengeScriptURL(interstitialHTML); got != "https://ct.captcha-delivery.com/interstitial.1.0.js" {
		t.Fatalf("ChallengeScriptURL = %q", got)
	}
	if got := ChallengeScriptURL(`<script>var ddm = {};</script>`); got != "" {
		t.Fatalf("inline page script URL = %q, want empty", got)
	}
	var got map[string]any
	srv := testServer(t, "/interstitial", `{"payload":"cid=x","session":"s"}`, &got)
	defer srv.Close()
	client, err := New(Config{Endpoint: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Session(Session{SessionKey: "s"}).GenerateInterstitial(context.Background(), &InterstitialInput{
		HTML: interstitialHTML, Script: "/*challenge script*/", DeviceLink: "https://geo.captcha-delivery.com/interstitial/?x=1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got["scriptHash"] != scriptHash("/*challenge script*/") || got["script"] != nil {
		t.Fatalf("wire scriptHash = %v script = %v, want the hash of the bytes the caller loaded, and not the bytes", got["scriptHash"], got["script"])
	}
}

func TestRequiredInputsFailBeforeTheNetwork(t *testing.T) {
	client, err := New(Config{Endpoint: "http://127.0.0.1:1"}) // nothing listens: any call would fail differently
	if err != nil {
		t.Fatal(err)
	}
	s := client.Session(Session{SessionKey: "s"})
	link := "https://geo.captcha-delivery.com/interstitial/?x=1"
	if _, err := s.GenerateInterstitial(context.Background(), &InterstitialInput{HTML: interstitialHTML}); err == nil || !strings.Contains(err.Error(), "DeviceLink is required") {
		t.Fatalf("missing DeviceLink: %v", err)
	}
	if _, err := s.GenerateInterstitial(context.Background(), &InterstitialInput{HTML: interstitialHTML, DeviceLink: link}); err == nil || !strings.Contains(err.Error(), "Script is required: load https://ct.captcha-delivery.com/interstitial.1.0.js") {
		t.Fatalf("missing Script: %v", err)
	}
	if _, err := s.GenerateSlider(context.Background(), &SliderInput{HTML: "<html></html>"}); err == nil || !strings.Contains(err.Error(), "CaptchaURL is required") {
		t.Fatalf("missing CaptchaURL: %v", err)
	}
}
